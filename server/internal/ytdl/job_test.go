package ytdl

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"synodl/server/internal/k8s"
)

func testConfig(mode Mode, raw string) JobConfig {
	return JobConfig{
		Namespace: "synodl",
		Image:     "jauderho/yt-dlp:2026.08.19",
		RequestID: "abc123",
		UserID:    "7",
		Mode:      mode,
		Libraries: map[Mode]Library{
			ModeMusic:      {ClaimName: "synodl-music", UID: 1000, GID: 1000},
			ModeMusicVideo: {ClaimName: "synodl-music-video", UID: 1000, GID: 1000},
		},
		DeadlineSeconds:    7200,
		TTLSeconds:         86400,
		MinDurationSeconds: 90,
	}
}

func buildOK(t *testing.T, mode Mode, raw string) *k8s.Job {
	t.Helper()
	tgt, err := Classify(raw)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	c := testConfig(mode, raw)
	c.Target = tgt
	j, err := BuildJob(c)
	if err != nil {
		t.Fatalf("BuildJob: %v", err)
	}
	return j
}

func TestBuildJob_Labels(t *testing.T) {
	j := buildOK(t, ModeMusic, "https://youtu.be/abc")
	want := map[string]string{
		LabelManagedBy: "synodl",
		LabelKind:      "ytdl",
		LabelRequestID: "abc123",
		LabelMode:      "music",
		LabelScope:     "single",
	}
	for k, v := range want {
		if got := j.Metadata.Labels[k]; got != v {
			t.Errorf("label %s = %q, want %q", k, got, v)
		}
	}
	// The URL is an ANNOTATION, not a label: label values are length-limited and
	// character-restricted, and a URL is neither.
	if j.Metadata.Annotations[AnnSourceURL] != "https://youtu.be/abc" {
		t.Errorf("source url annotation = %q", j.Metadata.Annotations[AnnSourceURL])
	}
	if j.Metadata.Annotations[AnnSubmittedBy] != "7" {
		t.Errorf("submitted-by annotation = %q", j.Metadata.Annotations[AnnSubmittedBy])
	}
	// Pod labels must match, or a LIST by selector would miss the pod.
	if j.Spec.Template.Metadata.Labels[LabelRequestID] != "abc123" {
		t.Error("pod template must carry the request id label")
	}
}

func TestBuildJob_IsEphemeral(t *testing.T) {
	j := buildOK(t, ModeMusic, "https://youtu.be/abc")
	ps := j.Spec.Template.Spec
	if ps.RestartPolicy != "Never" {
		t.Errorf("restartPolicy = %q, want Never", ps.RestartPolicy)
	}
	if j.Spec.BackoffLimit == nil || *j.Spec.BackoffLimit != 0 {
		t.Error("backoffLimit must be 0: a retry would repeat a partial download")
	}
	if j.Spec.ActiveDeadlineSeconds == nil || *j.Spec.ActiveDeadlineSeconds != 7200 {
		t.Error("activeDeadlineSeconds must be set so nothing runs forever (FR-020)")
	}
	if j.Spec.TTLSecondsAfterFinished == nil || *j.Spec.TTLSecondsAfterFinished != 86400 {
		t.Error("ttlSecondsAfterFinished must be set so workers sweep themselves")
	}
	if ps.AutomountServiceAccountToken == nil || *ps.AutomountServiceAccountToken {
		t.Error("a worker must not be given cluster credentials")
	}
}

func TestBuildJob_RunsAsTheLibraryOwner(t *testing.T) {
	j := buildOK(t, ModeMusic, "https://youtu.be/abc")
	sc := j.Spec.Template.Spec.SecurityContext
	if sc == nil || sc.RunAsUser == nil || *sc.RunAsUser != 1000 || sc.RunAsGroup == nil || *sc.RunAsGroup != 1000 {
		t.Error("worker must run as the library's uid/gid so files are not root-owned (FR-022)")
	}
	var sawCache bool
	for _, e := range j.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "XDG_CACHE_HOME" && e.Value == "/tmp" {
			sawCache = true
		}
	}
	if !sawCache {
		t.Error("XDG_CACHE_HOME must point somewhere writable: the pod has no home directory")
	}
}

// FR-006 made structural. A music job must not merely avoid *writing* the video
// library — it must not mount it, so the wrong path is unreachable rather than
// merely unused.
func TestBuildJob_MountsExactlyTheTargetLibrary(t *testing.T) {
	cases := []struct {
		mode      Mode
		wantClaim string
		notClaim  string
	}{
		{ModeMusic, "synodl-music", "synodl-music-video"},
		{ModeMusicVideo, "synodl-music-video", "synodl-music"},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			j := buildOK(t, tc.mode, "https://youtu.be/abc")
			ps := j.Spec.Template.Spec
			if len(ps.Volumes) != 1 {
				t.Fatalf("want exactly 1 volume, got %d", len(ps.Volumes))
			}
			if ps.Volumes[0].PersistentVolumeClaim == nil || ps.Volumes[0].PersistentVolumeClaim.ClaimName != tc.wantClaim {
				t.Fatalf("volume claim = %+v, want %q", ps.Volumes[0].PersistentVolumeClaim, tc.wantClaim)
			}
			mounts := ps.Containers[0].VolumeMounts
			if len(mounts) != 1 || mounts[0].MountPath != "/out" {
				t.Fatalf("want exactly one mount at /out, got %+v", mounts)
			}
			// The strongest form of the assertion: the other library's name must
			// not appear ANYWHERE in the serialised Job.
			//
			// Matched as a QUOTED JSON value, not as a bare substring. The real
			// claim names are "synodl-music" and "synodl-music-video", so one is
			// a prefix of the other and a naive Contains reports a false failure
			// on the very pair this is meant to protect.
			blob, err := json.Marshal(j)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(blob), strconv.Quote(tc.notClaim)) {
				t.Errorf("serialised job references the other library %q", tc.notClaim)
			}
		})
	}
}

func TestBuildJob_ArgsCarryTheURLOnceAndLast(t *testing.T) {
	j := buildOK(t, ModeMusic, "https://youtu.be/abc")
	args := j.Spec.Template.Spec.Containers[0].Args
	if len(args) < 2 || args[len(args)-1] != "https://youtu.be/abc" || args[len(args)-2] != "--" {
		t.Fatalf("args must end with `-- <url>`, got %v", args[max(0, len(args)-3):])
	}
	if len(j.Spec.Template.Spec.Containers[0].Command) == 0 {
		t.Error("command must be set explicitly rather than relying on the image entrypoint")
	}
}

func TestBuildJob_Rejects(t *testing.T) {
	tgt, _ := Classify("https://youtu.be/abc")

	t.Run("unknown mode", func(t *testing.T) {
		c := testConfig("bogus", "")
		c.Target = tgt
		if _, err := BuildJob(c); err == nil {
			t.Error("want error for an unknown mode")
		}
	})
	t.Run("no library configured for the mode", func(t *testing.T) {
		c := testConfig(ModeMusic, "")
		c.Target = tgt
		c.Libraries = map[Mode]Library{ModeMusicVideo: {ClaimName: "x"}}
		if _, err := BuildJob(c); err == nil {
			t.Error("want error when the target library is not configured")
		}
	})
	t.Run("no image", func(t *testing.T) {
		c := testConfig(ModeMusic, "")
		c.Target = tgt
		c.Image = ""
		if _, err := BuildJob(c); err == nil {
			t.Error("want error when no worker image is configured")
		}
	})
}

// The whole of FR-017/FR-018 lives in this table.
func TestStateOf(t *testing.T) {
	cond := func(typ, status, reason string) k8s.JobStatus {
		return k8s.JobStatus{Conditions: []k8s.JobCondition{{Type: typ, Status: status, Reason: reason}}}
	}
	cases := []struct {
		name string
		st   k8s.JobStatus
		want State
	}{
		{"created, nothing running yet", k8s.JobStatus{}, StateScheduled},
		{"pod running", k8s.JobStatus{Active: 1}, StateDownloading},
		{"succeeded count", k8s.JobStatus{Succeeded: 1}, StateCompleted},
		{"complete condition", cond("Complete", "True", ""), StateCompleted},
		{"failed count", k8s.JobStatus{Failed: 1}, StateFailed},
		{"failed condition", cond("Failed", "True", "BackoffLimitExceeded"), StateFailed},
		{"deadline exceeded is a failure, not a completion", cond("Failed", "True", "DeadlineExceeded"), StateFailed},
		{"false conditions are ignored", cond("Complete", "False", ""), StateScheduled},
		// If a job somehow reports both, failure wins: reporting a failed
		// download as completed is the one outcome FR-018 forbids.
		{"failure wins over success", k8s.JobStatus{Succeeded: 1, Failed: 1}, StateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StateOf(k8s.Job{Status: tc.st}); got != tc.want {
				t.Errorf("StateOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestState_Terminal(t *testing.T) {
	if !StateCompleted.Terminal() || !StateFailed.Terminal() {
		t.Error("completed and failed are terminal")
	}
	if StateScheduled.Terminal() || StateDownloading.Terminal() {
		t.Error("scheduled and started are not terminal")
	}
}

// The six states and the transitions between them (spec 0013, FR-013a/c).
func TestStateTransitions(t *testing.T) {
	for _, tc := range []struct {
		from, to State
		want     bool
		why      string
	}{
		{StateQueued, StateScheduled, true, "a queued download is admitted"},
		{StateScheduled, StateDownloading, true, "the worker starts"},
		{StateDownloading, StateCompleted, true, "it finishes"},
		{StateDownloading, StateFailed, true, "or it does not"},
		{StateResolving, StateQueued, true, "expansion produces items, each queued"},
		{StateResolving, StateFailed, true, "expansion can fail outright"},
		{StateFailed, StateQueued, true, "an explicit retry, the only way out of a final state"},

		{StateCompleted, StateQueued, false, "a completed download is never re-run"},
		{StateCompleted, StateDownloading, false, "nor resumed"},
		{StateFailed, StateDownloading, false, "a retry goes through the queue, never straight to a worker"},
		{StateQueued, StateDownloading, false, "nothing skips admission"},
		{StateDownloading, StateQueued, false, "a running download does not go back to waiting"},
		{StateScheduled, StateQueued, false, "once handed over, it is not un-handed"},
	} {
		if got := tc.from.CanTransitionTo(tc.to); got != tc.want {
			t.Errorf("%s → %s = %v, want %v (%s)", tc.from, tc.to, got, tc.want, tc.why)
		}
	}
}

func TestStateValid(t *testing.T) {
	for _, s := range []State{StateResolving, StateQueued, StateScheduled, StateDownloading, StateCompleted, StateFailed} {
		if !s.Valid() {
			t.Errorf("%q should be a valid state", s)
		}
	}
	for _, s := range []State{"", "started", "pending", "running"} {
		if State(s).Valid() {
			t.Errorf("%q should NOT be a valid state — 'started' in particular was spec 0012's name for downloading", s)
		}
	}
}
