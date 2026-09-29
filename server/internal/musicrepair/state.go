package musicrepair

import (
	"errors"

	"synodl/server/internal/k8s"
)

// Run states. `running` is a claim the database holds; the CLUSTER decides whether
// it is still true. A record that says running for a Job that is gone or failed is
// corrected here — the row never overrides what the cluster says.
const (
	StateRunning    = "running"
	StateFinished   = "finished"
	StateRefused    = "refused"
	StateUnfinished = "unfinished"
)

const (
	// FreshSeconds is how long a finished check may still be applied (clarified:
	// 24 hours from when it finished).
	FreshSeconds int64 = 24 * 3600
	// StartGrace is how long a freshly recorded run may have no Job yet before it
	// is called unfinished: the Job is created moments after the row.
	StartGrace int64 = 60
)

var (
	ErrNoPlan         = errors.New("no such plan")
	ErrExpired        = errors.New("that check is more than 24 hours old")
	ErrAlreadyApplied = errors.New("that plan was already applied")
	ErrNotUndoable    = errors.New("nothing to undo for that plan")
	ErrBusy           = errors.New("a repair is already running")
)

// Run is the stored record of one request, as this package needs to see it.
type Run struct {
	ID         string
	Kind       string // check | apply | undo
	PlanID     string
	State      string
	StartedAt  int64
	FinishedAt int64
	Summary    *Summary
}

// Phase is what a Job says about itself.
type Phase int

const (
	PhaseMissing Phase = iota
	PhaseRunning
	PhaseSucceeded
	PhaseFailed
)

// JobPhase reads a Job's status. A nil Job is PhaseMissing.
func JobPhase(j *k8s.Job) Phase {
	if j == nil {
		return PhaseMissing
	}
	for _, c := range j.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Complete":
			return PhaseSucceeded
		case "Failed":
			return PhaseFailed
		}
	}
	switch {
	case j.Status.Succeeded > 0:
		return PhaseSucceeded
	case j.Status.Failed > 0:
		return PhaseFailed
	}
	return PhaseRunning
}

// NextState is a run's state given what the cluster now says.
//
// The result event, when there is one, is what decides between "it ran" and "it
// declined": the tool exits non-zero both when a step failed (3) and when it
// refused (4), and a non-zero exit fails the Job either way — so a failed Job that
// DID print a result is not "did not finish".
func NextState(run Run, phase Phase, sum *Summary, now int64) string {
	if run.State != StateRunning {
		return run.State
	}
	switch phase {
	case PhaseRunning:
		return StateRunning
	case PhaseMissing:
		if now-run.StartedAt < StartGrace {
			return StateRunning
		}
		return StateUnfinished
	}
	if sum != nil {
		if !sum.OK && isRefusal(sum.Reason) {
			return StateRefused
		}
		return StateFinished
	}
	if phase == PhaseSucceeded {
		return StateFinished // it completed but its report was missing or unreadable
	}
	return StateUnfinished // failed with nothing to show: a deadline, an eviction, a crash
}

func isRefusal(reason string) bool {
	switch reason {
	case "locked", "no_plan", "no_space", "rejected":
		return true
	}
	return false
}

// PlanView is the plan an admin may act on: the newest successful check.
type PlanView struct {
	ID          string
	CheckedAt   int64
	ExpiresAt   int64
	Status      string // ready | expired | applying | applied | apply_unfinished | undone
	CanApply    bool
	CanContinue bool
	Summary     *Summary
}

// UndoView is the last applied repair that can still be undone.
type UndoView struct {
	PlanID    string
	AppliedAt int64
	CanUndo   bool
}

// Active returns the run in progress, if any. `runs` is newest first.
func Active(runs []Run) *Run {
	for i := range runs {
		if runs[i].State == StateRunning {
			return &runs[i]
		}
	}
	return nil
}

func newestCheck(runs []Run, planID string) *Run {
	for i := range runs {
		r := &runs[i]
		if r.Kind == "check" && r.State == StateFinished && r.Summary != nil && r.Summary.OK && r.Summary.PlanID != "" &&
			(planID == "" || r.Summary.PlanID == planID) {
			return r
		}
	}
	return nil
}

// appliedEffect is the newest apply for a plan that actually touched the library:
// one that finished or did not finish. A refused apply changed nothing.
func appliedEffect(runs []Run, planID string) *Run {
	for i := range runs {
		r := &runs[i]
		if r.Kind == "apply" && r.PlanID == planID && (r.State == StateFinished || r.State == StateUnfinished) {
			return r
		}
	}
	return nil
}

func undoneAfter(runs []Run, planID string, since int64) bool {
	for i := range runs {
		r := &runs[i]
		if r.Kind == "undo" && r.PlanID == planID && r.State == StateFinished && r.Summary != nil && r.Summary.OK && r.StartedAt >= since {
			return true
		}
	}
	return false
}

func cleanApply(r *Run) bool {
	return r.State == StateFinished && r.Summary != nil && r.Summary.OK &&
		(r.Summary.Apply == nil || r.Summary.Apply.Failed == 0)
}

// DerivePlan says what can be done with the newest successful check. `runs` is
// newest first. It returns nil when there is no such check.
func DerivePlan(runs []Run, now int64) *PlanView {
	chk := newestCheck(runs, "")
	if chk == nil {
		return nil
	}
	id := chk.Summary.PlanID
	p := &PlanView{ID: id, CheckedAt: chk.FinishedAt, ExpiresAt: chk.FinishedAt + FreshSeconds, Summary: chk.Summary}
	act := Active(runs)

	effect := appliedEffect(runs, id)
	switch {
	case act != nil && act.Kind == "apply" && act.PlanID == id:
		p.Status = "applying"
	case effect != nil && undoneAfter(runs, id, effect.StartedAt):
		p.Status = "undone"
	case effect != nil && cleanApply(effect):
		p.Status = "applied"
	case effect != nil:
		p.Status = "apply_unfinished"
	case now >= p.ExpiresAt:
		p.Status = "expired"
	default:
		p.Status = "ready"
	}
	p.CanApply = act == nil && p.Status == "ready"
	p.CanContinue = act == nil && p.Status == "apply_unfinished"
	return p
}

// DeriveUndo finds the last applied repair that has not been undone.
func DeriveUndo(runs []Run) *UndoView {
	for i := range runs {
		r := &runs[i]
		if r.Kind != "apply" || (r.State != StateFinished && r.State != StateUnfinished) {
			continue
		}
		if undoneAfter(runs, r.PlanID, r.StartedAt) {
			return nil
		}
		return &UndoView{PlanID: r.PlanID, AppliedAt: r.StartedAt, CanUndo: Active(runs) == nil}
	}
	return nil
}

// CheckApply is the guard an apply request passes before anything is started.
//
// The server holds only plan ids and summaries, so a plan it did not make — one
// made from the command line, say — is simply "no such plan": it cannot apply
// what it cannot see.
func CheckApply(runs []Run, planID string, now int64) error {
	if Active(runs) != nil {
		return ErrBusy
	}
	chk := newestCheck(runs, planID)
	if chk == nil {
		return ErrNoPlan
	}
	if effect := appliedEffect(runs, planID); effect != nil {
		if undoneAfter(runs, planID, effect.StartedAt) || cleanApply(effect) {
			return ErrAlreadyApplied
		}
		return nil // did not finish, or finished with failed steps: continue it
	}
	if now >= chk.FinishedAt+FreshSeconds {
		return ErrExpired
	}
	return nil
}

// CheckUndo is the guard an undo request passes.
func CheckUndo(runs []Run, planID string) error {
	if Active(runs) != nil {
		return ErrBusy
	}
	effect := appliedEffect(runs, planID)
	if effect == nil || undoneAfter(runs, planID, effect.StartedAt) {
		return ErrNotUndoable
	}
	return nil
}
