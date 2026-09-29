package musicrepair

import (
	"errors"
	"testing"

	"synodl/server/internal/k8s"
)

const hour = int64(3600)

func check(id string, finished int64, planID string) Run {
	return Run{ID: id, Kind: "check", PlanID: planID, State: StateFinished, StartedAt: finished - 600, FinishedAt: finished,
		Summary: &Summary{Kind: "check", OK: true, PlanID: planID, Check: &Check{Duplicates: 5}}}
}

func apply(id string, state string, at int64, planID string, failed int64) Run {
	r := Run{ID: id, Kind: "apply", PlanID: planID, State: state, StartedAt: at, FinishedAt: at + 100}
	if state == StateFinished {
		r.Summary = &Summary{Kind: "apply", OK: failed == 0, PlanID: planID, Apply: &Apply{Done: 10, Failed: failed}}
	}
	return r
}

func undo(id string, at int64, planID string) Run {
	return Run{ID: id, Kind: "undo", PlanID: planID, State: StateFinished, StartedAt: at, FinishedAt: at + 50,
		Summary: &Summary{Kind: "undo", OK: true, PlanID: planID, Undo: &Undo{Restored: 10}}}
}

func running(kind string, at int64) Run {
	return Run{ID: "run", Kind: kind, State: StateRunning, StartedAt: at}
}

// ---- deriving a run's state from the cluster --------------------------------

func job(active, succeeded, failed int32, cond ...string) *k8s.Job {
	j := &k8s.Job{Status: k8s.JobStatus{Active: active, Succeeded: succeeded, Failed: failed}}
	for _, c := range cond {
		j.Status.Conditions = append(j.Status.Conditions, k8s.JobCondition{Type: c, Status: "True"})
	}
	return j
}

func TestJobPhase(t *testing.T) {
	cases := []struct {
		name string
		j    *k8s.Job
		want Phase
	}{
		{"absent", nil, PhaseMissing},
		{"active", job(1, 0, 0), PhaseRunning},
		{"nothing yet", job(0, 0, 0), PhaseRunning},
		{"succeeded", job(0, 1, 0), PhaseSucceeded},
		{"complete condition", job(0, 0, 0, "Complete"), PhaseSucceeded},
		{"failed count", job(0, 0, 1), PhaseFailed},
		{"failed condition (deadline)", job(0, 0, 0, "Failed"), PhaseFailed},
	}
	for _, tc := range cases {
		if got := JobPhase(tc.j); got != tc.want {
			t.Errorf("%s: phase = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNextState(t *testing.T) {
	now := int64(10_000)
	ok := &Summary{Kind: "check", OK: true}
	refused := &Summary{Kind: "apply", OK: false, Reason: "locked"}
	stepsFailed := &Summary{Kind: "apply", OK: false, Reason: "failed_steps", Apply: &Apply{Failed: 3}}
	run := running("check", now-100)
	cases := []struct {
		name  string
		run   Run
		phase Phase
		sum   *Summary
		want  string
	}{
		{"still running", run, PhaseRunning, nil, StateRunning},
		{"completed with a report", run, PhaseSucceeded, ok, StateFinished},
		{"completed with no report", run, PhaseSucceeded, nil, StateFinished},
		{"the tool refused (exit 4 fails the Job)", run, PhaseFailed, refused, StateRefused},
		{"steps failed (exit 3 fails the Job) but it did finish", run, PhaseFailed, stepsFailed, StateFinished},
		{"failed with no report: deadline, eviction, crash", run, PhaseFailed, nil, StateUnfinished},
		{"vanished within the grace period", running("check", now-10), PhaseMissing, nil, StateRunning},
		{"vanished after the grace period", running("check", now-StartGrace-1), PhaseMissing, nil, StateUnfinished},
		{"a finished record is never rewritten", Run{State: StateFinished}, PhaseFailed, nil, StateFinished},
	}
	for _, tc := range cases {
		if got := NextState(tc.run, tc.phase, tc.sum, now); got != tc.want {
			t.Errorf("%s: state = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// ---- what may be applied ------------------------------------------------------

const planA = "20260929T071341Z-73c844"

func TestDerivePlan_ReadyThenExpired(t *testing.T) {
	finished := int64(1_000_000)
	runs := []Run{check("c1", finished, planA)}
	for _, tc := range []struct {
		name   string
		now    int64
		status string
		can    bool
	}{
		{"just finished", finished, "ready", true},
		{"23h59m", finished + 24*hour - 60, "ready", true},
		{"exactly 24h", finished + 24*hour, "expired", false},
		{"24h01m", finished + 24*hour + 60, "expired", false},
	} {
		p := DerivePlan(runs, tc.now)
		if p == nil || p.ID != planA || p.Status != tc.status || p.CanApply != tc.can {
			t.Errorf("%s: plan = %+v, want %s canApply=%v", tc.name, p, tc.status, tc.can)
		}
		if p != nil && p.ExpiresAt != finished+24*hour {
			t.Errorf("%s: expiresAt = %d, want the check's finish + 24h", tc.name, p.ExpiresAt)
		}
	}
}

func TestDerivePlan_OnlyASuccessfulCheckWithAPlanCounts(t *testing.T) {
	failed := check("c", 100, planA)
	failed.State = StateUnfinished
	noPlan := check("c", 100, "")
	refused := check("c", 100, planA)
	refused.State = StateRefused
	notOK := check("c", 100, planA)
	notOK.Summary.OK = false
	for name, r := range map[string]Run{"unfinished": failed, "no plan id": noPlan, "refused": refused, "not ok": notOK} {
		if p := DerivePlan([]Run{r}, 200); p != nil {
			t.Errorf("%s: plan = %+v, want none", name, p)
		}
	}
	if p := DerivePlan(nil, 1); p != nil {
		t.Errorf("no runs: plan = %+v", p)
	}
}

func TestDerivePlan_TheNewestCheckWins(t *testing.T) {
	runs := []Run{check("new", 2000, "20260929T080000Z-aaaaaa"), check("old", 1000, planA)}
	if p := DerivePlan(runs, 3000); p.ID != "20260929T080000Z-aaaaaa" {
		t.Errorf("plan = %s, want the newest check's", p.ID)
	}
}

func TestDerivePlan_AfterAnApply(t *testing.T) {
	c := check("c", 1000, planA)
	cases := []struct {
		name   string
		runs   []Run
		status string
		apply  bool
		cont   bool
	}{
		{"applied cleanly", []Run{apply("a", StateFinished, 1100, planA, 0), c}, "applied", false, false},
		{"applied with failed steps: can continue", []Run{apply("a", StateFinished, 1100, planA, 3), c}, "apply_unfinished", false, true},
		{"apply did not finish: can continue", []Run{apply("a", StateUnfinished, 1100, planA, 0), c}, "apply_unfinished", false, true},
		{"apply was refused: still ready", []Run{apply("a", StateRefused, 1100, planA, 0), c}, "ready", true, false},
		{"applied then undone", []Run{undo("u", 1300, planA), apply("a", StateFinished, 1100, planA, 0), c}, "undone", false, false},
		{"apply running", []Run{running("apply", 1100), c}, "applying", false, false},
	}
	for _, tc := range cases {
		if tc.name == "apply running" {
			tc.runs[0].PlanID = planA
		}
		p := DerivePlan(tc.runs, 1500)
		if p.Status != tc.status || p.CanApply != tc.apply || p.CanContinue != tc.cont {
			t.Errorf("%s: plan = %+v, want status=%s canApply=%v canContinue=%v", tc.name, p, tc.status, tc.apply, tc.cont)
		}
	}
}

func TestDerivePlan_ContinuingIsAllowedAfterTheWindowBecauseItHasAlreadyBegun(t *testing.T) {
	runs := []Run{apply("a", StateUnfinished, 1100, planA, 0), check("c", 1000, planA)}
	p := DerivePlan(runs, 1000+30*hour)
	if p.Status != "apply_unfinished" || !p.CanContinue {
		t.Errorf("plan = %+v: an apply already under way is resumed, not aged out (the tool still skips anything that changed)", p)
	}
}

func TestDerivePlan_NothingCanBeStartedWhileARunIsActive(t *testing.T) {
	runs := []Run{running("check", 1500), check("c", 1000, planA)}
	if p := DerivePlan(runs, 1600); p.CanApply || p.CanContinue {
		t.Errorf("plan = %+v, want nothing offered while a run is active", p)
	}
}

// ---- undo ---------------------------------------------------------------------

func TestDeriveUndo(t *testing.T) {
	c := check("c", 1000, planA)
	cases := []struct {
		name string
		runs []Run
		want bool
	}{
		{"never applied", []Run{c}, false},
		{"applied", []Run{apply("a", StateFinished, 1100, planA, 0), c}, true},
		{"apply did not finish (the tool restores what its journal recorded)", []Run{apply("a", StateUnfinished, 1100, planA, 0), c}, true},
		{"apply was refused: nothing to undo", []Run{apply("a", StateRefused, 1100, planA, 0), c}, false},
		{"already undone", []Run{undo("u", 1300, planA), apply("a", StateFinished, 1100, planA, 0), c}, false},
		{"applied again after an undo", []Run{apply("a2", StateFinished, 1500, planA, 0), undo("u", 1300, planA), apply("a", StateFinished, 1100, planA, 0), c}, true},
		{"a run is active", []Run{running("apply", 1200), apply("a", StateFinished, 1100, planA, 0), c}, false},
	}
	for _, tc := range cases {
		u := DeriveUndo(tc.runs)
		got := u != nil && u.CanUndo
		if got != tc.want {
			t.Errorf("%s: undo = %+v, want canUndo=%v", tc.name, u, tc.want)
		}
	}
	u := DeriveUndo([]Run{apply("a", StateFinished, 1100, planA, 0), c})
	if u.PlanID != planA || u.AppliedAt != 1100 {
		t.Errorf("undo = %+v, want the plan and time of the last apply", u)
	}
}

// ---- the guards the handlers use ----------------------------------------------

func TestCheckApply(t *testing.T) {
	c := check("c", 1000, planA)
	now := int64(1000 + 5*hour)
	cases := []struct {
		name string
		runs []Run
		plan string
		now  int64
		want error
	}{
		{"fresh and unapplied", []Run{c}, planA, now, nil},
		{"unknown plan (also: one made from the command line)", []Run{c}, "20260929T090000Z-bbbbbb", now, ErrNoPlan},
		{"no runs at all", nil, planA, now, ErrNoPlan},
		{"expired", []Run{c}, planA, 1000 + 24*hour, ErrExpired},
		{"already applied", []Run{apply("a", StateFinished, 1100, planA, 0), c}, planA, now, ErrAlreadyApplied},
		{"applied with failures may be continued", []Run{apply("a", StateFinished, 1100, planA, 2), c}, planA, now, nil},
		{"unfinished may be continued even after the window", []Run{apply("a", StateUnfinished, 1100, planA, 0), c}, planA, 1000 + 40*hour, nil},
		{"undone plans are not re-applied", []Run{undo("u", 1300, planA), apply("a", StateFinished, 1100, planA, 0), c}, planA, now, ErrAlreadyApplied},
		{"busy", []Run{running("check", 1500), c}, planA, now, ErrBusy},
	}
	for _, tc := range cases {
		if err := CheckApply(tc.runs, tc.plan, tc.now); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestCheckUndo(t *testing.T) {
	c := check("c", 1000, planA)
	if err := CheckUndo([]Run{apply("a", StateFinished, 1100, planA, 0), c}, planA); err != nil {
		t.Errorf("undo of an applied plan: %v", err)
	}
	for name, runs := range map[string][]Run{
		"never applied":  {c},
		"already undone": {undo("u", 1300, planA), apply("a", StateFinished, 1100, planA, 0), c},
		"busy":           {running("apply", 1200), apply("a", StateFinished, 1100, planA, 0), c},
	} {
		if err := CheckUndo(runs, planA); err == nil {
			t.Errorf("%s: undo was allowed", name)
		}
	}
	if err := CheckUndo([]Run{apply("a", StateFinished, 1100, planA, 0), c}, "20260929T090000Z-bbbbbb"); !errors.Is(err, ErrNotUndoable) {
		t.Errorf("a plan that was not applied: %v", err)
	}
}
