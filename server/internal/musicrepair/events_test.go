package musicrepair

import (
	"fmt"
	"strings"
	"testing"
)

// The worker's output is user data (constitution v2.2.0): produced by a program
// reading a library full of other people's file names. So the parser is tested
// as a boundary — hostile and broken input in, a fixed shape or nothing out.

const goodPlanID = "20260929T071341Z-73c844"

// ev builds one event line. The literals below are wrapped for reading; a real
// event is always a single line, so raw newlines and tabs are removed here.
func ev(json string) string {
	return "@@synodl " + strings.NewReplacer("\n", "", "\t", "").Replace(json) + "\n"
}

func TestParseLog_ValidProgressAndResult(t *testing.T) {
	raw := "[plan] scanning /library\n" +
		ev(`{"event":"progress","phase":"lookup","done":1275,"total":4017}`) +
		"[plan] looked up 1275/4017\n" +
		ev(`{"event":"result","kind":"check","ok":true,"planId":"`+goodPlanID+`","planFile":".repair/plan-x.md",
		"check":{"tracks":5795,"songs":4017,"duplicates":1774,"moves":3970,"retags":4017,"covers":886,"playlists":88,
		"conflicts":0,"nameClashes":52,"matched":1828,"noMatch":2186,"notLookedUp":3,"toSingles":2917,"albumKnown":886,
		"orphanNfo":3484,"bytesReclaimed":13368173185,"bytesNeeded":548758400,"freeBytes":4600000000000,
		"leftAlone":{"total":6,"byReason":[{"reason":"no video id","count":4}],
		"examples":[{"path":"Avaria/Singles/Hold Me Down.mp3","reason":"no video id"}]}}}`)
	p, s := ParseLog([]byte(raw))
	if p == nil || p.Phase != "lookup" || p.Done != 1275 || p.Total != 4017 {
		t.Fatalf("progress = %+v", p)
	}
	if s == nil || s.Kind != "check" || !s.OK || s.PlanID != goodPlanID || s.PlanFile != ".repair/plan-x.md" {
		t.Fatalf("summary = %+v", s)
	}
	c := s.Check
	if c == nil || c.Duplicates != 1774 || c.Moves != 3970 || c.FreeBytes != 4600000000000 || c.LeftAlone.Total != 6 {
		t.Fatalf("check = %+v", c)
	}
	if len(c.LeftAlone.ByReason) != 1 || c.LeftAlone.ByReason[0].Reason != "no video id" || len(c.LeftAlone.Examples) != 1 {
		t.Fatalf("left alone = %+v", c.LeftAlone)
	}
}

func TestParseLog_OnlyPrefixedLinesCount(t *testing.T) {
	raw := `{"event":"result","kind":"check","ok":true}` + "\n" + // JSON without the prefix
		" @@synodl {\"event\":\"result\",\"kind\":\"check\",\"ok\":true}\n" + // prefix not at the start
		"[apply] @@synodl {\"event\":\"result\",\"kind\":\"check\",\"ok\":true}\n"
	if p, s := ParseLog([]byte(raw)); p != nil || s != nil {
		t.Fatalf("got %+v %+v, want nothing: the prefix must START the line", p, s)
	}
}

func TestParseLog_BrokenLinesAreDropped(t *testing.T) {
	for name, line := range map[string]string{
		"malformed json":   ev(`{"event":"result",`),
		"array not object": ev(`[1,2,3]`),
		"scalar":           ev(`"result"`),
		"unknown event":    ev(`{"event":"explode","kind":"check","ok":true}`),
		"unknown kind":     ev(`{"event":"result","kind":"nuke","ok":true}`),
		"unknown phase":    ev(`{"event":"progress","phase":"bogus","done":1,"total":2}`),
		"string as number": ev(`{"event":"progress","phase":"scan","done":"lots","total":2}`),
		"no event":         ev(`{"kind":"check","ok":true}`),
	} {
		p, s := ParseLog([]byte(line))
		if p != nil || s != nil {
			t.Errorf("%s: got %+v %+v, want the line dropped", name, p, s)
		}
	}
}

func TestParseLog_LineCaps(t *testing.T) {
	long := strings.Repeat("a", 5000)
	if p, _ := ParseLog([]byte(ev(`{"event":"progress","phase":"scan","done":1,"total":2,"x":"` + long + `"}`))); p != nil {
		t.Error("a progress line over 4 KiB must be dropped")
	}
	huge := strings.Repeat("a", 17000)
	if _, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"x":"` + huge + `"}`))); s != nil {
		t.Error("a result line over 16 KiB must be dropped")
	}
	// A result line of 9 KB is legitimate (20 long paths) and must survive.
	ex := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		ex = append(ex, fmt.Sprintf(`{"path":"%s","reason":"%s"}`, strings.Repeat("p", 200), strings.Repeat("r", 120)))
	}
	line := ev(`{"event":"result","kind":"check","ok":true,"check":{"leftAlone":{"total":20,"examples":[` + strings.Join(ex, ",") + `]}}}`)
	if _, s := ParseLog([]byte(line)); s == nil || s.Check == nil || len(s.Check.LeftAlone.Examples) != 20 {
		t.Errorf("a full-size legitimate result was dropped: %+v", s)
	}
}

func TestParseLog_NumbersAreClamped(t *testing.T) {
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"check":
	  {"tracks":-5,"songs":1e30,"duplicates":3.9,"moves":9223372036854775807,"freeBytes":4600000000000}}`)))
	if s == nil || s.Check == nil {
		t.Fatalf("summary = %+v", s)
	}
	c := s.Check
	if c.Tracks != 0 {
		t.Errorf("negative = %d, want 0", c.Tracks)
	}
	if c.Songs != MaxInt || c.Moves != MaxInt {
		t.Errorf("huge = %d / %d, want clamped to %d", c.Songs, c.Moves, MaxInt)
	}
	if c.Duplicates != 3 {
		t.Errorf("3.9 = %d, want 3", c.Duplicates)
	}
	if c.FreeBytes != 4600000000000 {
		t.Errorf("a real 4.6 TB free-space figure was altered: %d", c.FreeBytes)
	}
}

func TestParseLog_StringsAreCleanedAndCapped(t *testing.T) {
	evil := "a\\nb\\u0000c\\u2028d\\u007fe"
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"planFile":"` + strings.Repeat("f", 500) + `",
	  "check":{"leftAlone":{"total":1,"byReason":[{"reason":"` + strings.Repeat("r", 500) + `","count":1}],
	  "examples":[{"path":"` + evil + strings.Repeat("p", 500) + `","reason":"` + evil + `"}]}}}`)))
	if s == nil || s.Check == nil {
		t.Fatalf("summary = %+v", s)
	}
	if n := len([]rune(s.PlanFile)); n != PathCap {
		t.Errorf("planFile length = %d, want capped at %d", n, PathCap)
	}
	if n := len([]rune(s.Check.LeftAlone.ByReason[0].Reason)); n != ReasonCap {
		t.Errorf("reason length = %d, want capped at %d", n, ReasonCap)
	}
	ex := s.Check.LeftAlone.Examples[0]
	for _, r := range ex.Path + ex.Reason {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			t.Fatalf("control character %U survived in %q / %q", r, ex.Path, ex.Reason)
		}
	}
}

func TestParseLog_ArraysAreCapped(t *testing.T) {
	var reasons, examples, notes []string
	for i := 0; i < 50; i++ {
		reasons = append(reasons, fmt.Sprintf(`{"reason":"r%d","count":%d}`, i, i))
		examples = append(examples, fmt.Sprintf(`{"path":"a/%d.mp3","reason":"x"}`, i))
		notes = append(notes, fmt.Sprintf(`{"path":"a/%d.mp3","note":"x"}`, i))
	}
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"check":{"leftAlone":{"total":50,
	  "byReason":[` + strings.Join(reasons, ",") + `],"examples":[` + strings.Join(examples, ",") + `]}}}`)))
	if s.Check == nil || len(s.Check.LeftAlone.ByReason) != MaxReasons || len(s.Check.LeftAlone.Examples) != MaxCheckExamples {
		t.Fatalf("check arrays = %d / %d, want %d / %d", len(s.Check.LeftAlone.ByReason), len(s.Check.LeftAlone.Examples), MaxReasons, MaxCheckExamples)
	}
	_, s = ParseLog([]byte(ev(`{"event":"result","kind":"apply","ok":true,"apply":{"done":1,"skippedByReason":[` +
		strings.Join(reasons, ",") + `],"failedExamples":[` + strings.Join(notes, ",") + `]}}`)))
	if len(s.Apply.SkippedByReason) != MaxReasons || len(s.Apply.FailedExamples) != MaxOutcomeExamples {
		t.Fatalf("apply arrays = %d / %d", len(s.Apply.SkippedByReason), len(s.Apply.FailedExamples))
	}
	_, s = ParseLog([]byte(ev(`{"event":"result","kind":"undo","ok":true,"undo":{"restored":1,"skippedExamples":[` + strings.Join(notes, ",") + `]}}`)))
	if len(s.Undo.SkippedExamples) != MaxOutcomeExamples {
		t.Fatalf("undo examples = %d", len(s.Undo.SkippedExamples))
	}
}

func TestParseLog_PlanIDMustMatchThePattern(t *testing.T) {
	for _, id := range []string{"../../etc", "20260929T071341Z-73C844", "x", "20260929T071341Z-73c8445", "20260929T071341Z-73c84;", " " + goodPlanID} {
		_, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"planId":"` + id + `"}`)))
		if s == nil {
			t.Fatalf("%q: a bad plan id must not discard the whole result", id)
		}
		if s.PlanID != "" {
			t.Errorf("planId %q survived as %q", id, s.PlanID)
		}
	}
}

func TestParseLog_UnknownReasonIsDropped(t *testing.T) {
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"apply","ok":false,"reason":"because I said so"}`)))
	if s == nil || s.OK || s.Reason != "" {
		t.Fatalf("summary = %+v, want a refusal with no free-text reason", s)
	}
	_, s = ParseLog([]byte(ev(`{"event":"result","kind":"apply","ok":false,"reason":"locked"}`)))
	if s.Reason != "locked" {
		t.Errorf("reason = %q, want the fixed value kept", s.Reason)
	}
}

func TestParseLog_OnlyTheSectionMatchingTheKindIsKept(t *testing.T) {
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"check","ok":true,"check":{"tracks":1},"apply":{"done":9},"undo":{"restored":9}}`)))
	if s.Check == nil || s.Apply != nil || s.Undo != nil {
		t.Fatalf("summary = %+v, want only the check section", s)
	}
}

func TestParseLog_LastValidEventsWin(t *testing.T) {
	raw := ev(`{"event":"progress","phase":"scan","done":1,"total":9}`) +
		ev(`{"event":"result","kind":"check","ok":false,"reason":"locked"}`) +
		ev(`{"event":"progress","phase":"lookup","done":5,"total":9}`) +
		ev(`{"event":"progress","phase":"bogus","done":6,"total":9}`) + // invalid: must not replace
		ev(`{"event":"result","kind":"check","ok":true}`)
	p, s := ParseLog([]byte(raw))
	if p.Phase != "lookup" || p.Done != 5 {
		t.Errorf("progress = %+v, want the last VALID one", p)
	}
	if !s.OK {
		t.Errorf("summary = %+v, want the last valid result", s)
	}
}

func TestParseLog_OnlyTheLast64EventLinesAreConsidered(t *testing.T) {
	var b strings.Builder
	b.WriteString(ev(`{"event":"result","kind":"check","ok":true}`))
	for i := 0; i < 70; i++ {
		b.WriteString(ev(fmt.Sprintf(`{"event":"progress","phase":"lookup","done":%d,"total":100}`, i)))
	}
	p, s := ParseLog([]byte(b.String()))
	if s != nil {
		t.Error("a result more than 64 event lines from the end must not be found: the bound is on work done")
	}
	if p == nil || p.Done != 69 {
		t.Errorf("progress = %+v, want the last one", p)
	}
}

func TestParseLog_EmptyAndGarbage(t *testing.T) {
	for _, in := range [][]byte{nil, {}, []byte("\n\n\n"), []byte("plain text only\nmore text"), {0xff, 0xfe, 0x00, '@', '@'}} {
		if p, s := ParseLog(in); p != nil || s != nil {
			t.Errorf("input %q → %+v %+v, want nothing", in, p, s)
		}
	}
}

func TestParseLog_ARefusalCarriesNoSection(t *testing.T) {
	_, s := ParseLog([]byte(ev(`{"event":"result","kind":"apply","ok":false,"reason":"no_space"}`)))
	if s == nil || s.OK || s.Reason != "no_space" || s.Apply != nil {
		t.Fatalf("summary = %+v", s)
	}
}
