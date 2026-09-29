// Package musicrepair is the server's half of running the music library repair
// from Settings (spec 1053): building the worker Job, reading what the worker
// reports, and the rules for what an admin may do next. It has no HTTP and no
// database in it, so all of it is table-testable.
package musicrepair

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Prefix starts every line the worker wants the server to read.
const Prefix = "@@synodl "

// Bounds on what the server will accept. They mirror what the worker itself
// enforces (scripts/music_repair/events.py); both ends holding the same numbers is
// deliberate — the reader must never have to trust the writer.
const (
	// MaxInt bounds every count and byte figure. 10^15 covers a petabyte volume.
	MaxInt int64 = 1_000_000_000_000_000

	PathCap   = 200
	ReasonCap = 120

	MaxReasons         = 12
	MaxCheckExamples   = 20
	MaxOutcomeExamples = 10

	maxProgressLine = 4096
	maxResultLine   = 16384
	// maxEventLines is how many of the LAST event lines are considered. A run's
	// result is the last thing it prints, so the bound costs nothing real and caps
	// the work a hostile or runaway log can cause.
	maxEventLines = 64
)

// PlanIDPattern is what a plan id looks like. The same pattern guards a plan id
// coming back from the worker and one coming in from a request, because either
// can end up as an argv element of a Job (constitution: discrete argv, and only
// values that were validated first).
var PlanIDPattern = regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{6}$`)

var (
	phases  = map[string]bool{"scan": true, "lookup": true, "plan": true, "apply": true, "restore": true}
	kinds   = map[string]bool{"check": true, "apply": true, "undo": true}
	reasons = map[string]bool{"locked": true, "no_plan": true, "no_space": true, "rejected": true, "failed_steps": true}
)

// Progress is a run's latest reading. It lives in memory and is lost on restart
// without consequence.
type Progress struct {
	Phase string `json:"phase"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

type CountReason struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type Example struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type NoteExample struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

type LeftAlone struct {
	Total    int64         `json:"total"`
	ByReason []CountReason `json:"byReason"`
	Examples []Example     `json:"examples"`
}

// Check is what a plan would do, as numbers.
type Check struct {
	Tracks         int64     `json:"tracks"`
	Songs          int64     `json:"songs"`
	Duplicates     int64     `json:"duplicates"`
	Moves          int64     `json:"moves"`
	Retags         int64     `json:"retags"`
	Covers         int64     `json:"covers"`
	Playlists      int64     `json:"playlists"`
	Conflicts      int64     `json:"conflicts"`
	NameClashes    int64     `json:"nameClashes"`
	Matched        int64     `json:"matched"`
	NoMatch        int64     `json:"noMatch"`
	NotLookedUp    int64     `json:"notLookedUp"`
	ToSingles      int64     `json:"toSingles"`
	AlbumKnown     int64     `json:"albumKnown"`
	OrphanNfo      int64     `json:"orphanNfo"`
	BytesReclaimed int64     `json:"bytesReclaimed"`
	BytesNeeded    int64     `json:"bytesNeeded"`
	FreeBytes      int64     `json:"freeBytes"`
	LeftAlone      LeftAlone `json:"leftAlone"`
}

type Apply struct {
	Done            int64         `json:"done"`
	Skipped         int64         `json:"skipped"`
	Failed          int64         `json:"failed"`
	AlreadyDone     int64         `json:"alreadyDone"`
	SkippedByReason []CountReason `json:"skippedByReason"`
	FailedExamples  []NoteExample `json:"failedExamples"`
}

type Undo struct {
	Restored        int64         `json:"restored"`
	Skipped         int64         `json:"skipped"`
	SkippedExamples []NoteExample `json:"skippedExamples"`
}

// Summary is the ONLY view of a finished run the server ever holds. Exactly the
// section matching Kind can be present.
type Summary struct {
	Kind     string `json:"kind"`
	OK       bool   `json:"ok"`
	Reason   string `json:"reason,omitempty"`
	PlanID   string `json:"planId,omitempty"`
	PlanFile string `json:"planFile,omitempty"`
	Check    *Check `json:"check,omitempty"`
	Apply    *Apply `json:"apply,omitempty"`
	Undo     *Undo  `json:"undo,omitempty"`
}

// num decodes a JSON number and clamps it to [0, MaxInt]. Anything that is not a
// number is an error, which drops the whole line: a string where a count belongs
// means the writer is not the writer we expect.
type num int64

func (n *num) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || s == "" || s[0] == '"' {
		return errors.New("not a number")
	}
	switch {
	case math.IsNaN(f) || f < 0:
		*n = 0
	case f > float64(MaxInt):
		*n = num(MaxInt)
	default:
		*n = num(int64(f))
	}
	return nil
}

type rawCountReason struct {
	Reason string `json:"reason"`
	Count  num    `json:"count"`
}
type rawExample struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Note   string `json:"note"`
}
type rawLeftAlone struct {
	Total    num              `json:"total"`
	ByReason []rawCountReason `json:"byReason"`
	Examples []rawExample     `json:"examples"`
}
type rawCheck struct {
	Tracks, Songs, Duplicates, Moves, Retags, Covers, Playlists, Conflicts num
	NameClashes, Matched, NoMatch, NotLookedUp, ToSingles, AlbumKnown      num
	OrphanNfo, BytesReclaimed, BytesNeeded, FreeBytes                      num
	LeftAlone                                                              rawLeftAlone
}
type rawApply struct {
	Done, Skipped, Failed, AlreadyDone num
	SkippedByReason                    []rawCountReason
	FailedExamples                     []rawExample
}
type rawUndo struct {
	Restored, Skipped num
	SkippedExamples   []rawExample
}
type rawEvent struct {
	Event    string    `json:"event"`
	Phase    string    `json:"phase"`
	Kind     string    `json:"kind"`
	OK       bool      `json:"ok"`
	Reason   string    `json:"reason"`
	PlanID   string    `json:"planId"`
	PlanFile string    `json:"planFile"`
	Done     num       `json:"done"`
	Total    num       `json:"total"`
	Check    *rawCheck `json:"check"`
	Apply    *rawApply `json:"apply"`
	Undo     *rawUndo  `json:"undo"`
}

// ParseLog reads a worker's output and returns the latest valid progress and the
// latest valid result, either of which may be nil.
//
// It is a boundary: whatever is in `raw` — including bytes an attacker chose —
// yields a value of a fixed shape with every bound applied, or nothing. It never
// returns a substring of the input that was not cleaned and capped, and it never
// logs any of it.
func ParseLog(raw []byte) (*Progress, *Summary) {
	var lines [][]byte
	for _, l := range bytes.Split(raw, []byte("\n")) {
		if bytes.HasPrefix(l, []byte(Prefix)) {
			lines = append(lines, l[len(Prefix):])
		}
	}
	if len(lines) > maxEventLines {
		lines = lines[len(lines)-maxEventLines:]
	}

	var prog *Progress
	var sum *Summary
	for _, l := range lines {
		l = bytes.TrimRight(l, "\r")
		if len(l) > maxResultLine {
			continue
		}
		var e rawEvent
		if err := json.Unmarshal(l, &e); err != nil {
			continue
		}
		switch e.Event {
		case "progress":
			if len(l) > maxProgressLine || !phases[e.Phase] {
				continue
			}
			prog = &Progress{Phase: e.Phase, Done: int64(e.Done), Total: int64(e.Total)}
		case "result":
			if s := buildSummary(e); s != nil {
				sum = s
			}
		}
	}
	return prog, sum
}

func buildSummary(e rawEvent) *Summary {
	if !kinds[e.Kind] {
		return nil
	}
	s := &Summary{Kind: e.Kind, OK: e.OK}
	if reasons[e.Reason] {
		s.Reason = e.Reason
	}
	if PlanIDPattern.MatchString(e.PlanID) {
		s.PlanID = e.PlanID
	}
	s.PlanFile = clean(e.PlanFile, PathCap)
	switch e.Kind {
	case "check":
		if c := e.Check; c != nil {
			s.Check = &Check{
				Tracks: int64(c.Tracks), Songs: int64(c.Songs), Duplicates: int64(c.Duplicates), Moves: int64(c.Moves),
				Retags: int64(c.Retags), Covers: int64(c.Covers), Playlists: int64(c.Playlists),
				Conflicts: int64(c.Conflicts), NameClashes: int64(c.NameClashes), Matched: int64(c.Matched),
				NoMatch: int64(c.NoMatch), NotLookedUp: int64(c.NotLookedUp), ToSingles: int64(c.ToSingles),
				AlbumKnown: int64(c.AlbumKnown), OrphanNfo: int64(c.OrphanNfo),
				BytesReclaimed: int64(c.BytesReclaimed), BytesNeeded: int64(c.BytesNeeded), FreeBytes: int64(c.FreeBytes),
				LeftAlone: LeftAlone{
					Total:    int64(c.LeftAlone.Total),
					ByReason: countReasons(c.LeftAlone.ByReason),
					Examples: examples(c.LeftAlone.Examples),
				},
			}
		}
	case "apply":
		if a := e.Apply; a != nil {
			s.Apply = &Apply{
				Done: int64(a.Done), Skipped: int64(a.Skipped), Failed: int64(a.Failed), AlreadyDone: int64(a.AlreadyDone),
				SkippedByReason: countReasons(a.SkippedByReason), FailedExamples: noteExamples(a.FailedExamples),
			}
		}
	case "undo":
		if u := e.Undo; u != nil {
			s.Undo = &Undo{Restored: int64(u.Restored), Skipped: int64(u.Skipped), SkippedExamples: noteExamples(u.SkippedExamples)}
		}
	}
	return s
}

func countReasons(in []rawCountReason) []CountReason {
	out := []CountReason{}
	for _, r := range in {
		if len(out) == MaxReasons {
			break
		}
		out = append(out, CountReason{Reason: clean(r.Reason, ReasonCap), Count: int64(r.Count)})
	}
	return out
}

func examples(in []rawExample) []Example {
	out := []Example{}
	for _, x := range in {
		if len(out) == MaxCheckExamples {
			break
		}
		out = append(out, Example{Path: clean(x.Path, PathCap), Reason: clean(x.Reason, ReasonCap)})
	}
	return out
}

func noteExamples(in []rawExample) []NoteExample {
	out := []NoteExample{}
	for _, x := range in {
		if len(out) == MaxOutcomeExamples {
			break
		}
		out = append(out, NoteExample{Path: clean(x.Path, PathCap), Note: clean(x.Note, ReasonCap)})
	}
	return out
}

// clean makes a worker-supplied string safe to hold and to show: control
// characters (including the Unicode line and paragraph separators) become
// spaces, and it is cut to `cap` characters. It is deliberately not "escaped":
// nothing here is rendered as markup, and the client shows it as text.
func clean(s string, cap int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == cap {
			break
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}
