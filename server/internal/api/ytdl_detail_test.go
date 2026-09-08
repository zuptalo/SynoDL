package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

type ytdlDetailResp struct {
	RequestID  string   `json:"requestId"`
	Kind       string   `json:"kind"`
	URL        string   `json:"url"`
	Title      string   `json:"title"`
	Uploader   string   `json:"uploader"`
	Mode       string   `json:"mode"`
	Scope      string   `json:"scope"`
	State      string   `json:"state"`
	Progress   *float64 `json:"progress"`
	HasLyrics  *bool    `json:"hasLyrics"`
	LyricsLang string   `json:"lyricsLang"`
	Attempts   int      `json:"attempts"`
	SubmitBy   string   `json:"submittedBy"`
	CreatedAt  int64    `json:"submittedAt"`
	FinishedAt *int64   `json:"finishedAt"`
	Reason     string   `json:"reason"`
}

func ytdlDetail(t *testing.T, h http.Handler, auth map[string]string, id string) (ytdlDetailResp, int) {
	t.Helper()
	rec := do(t, h, "GET", "/v1/ytdl/"+id, "", auth)
	var out ytdlDetailResp
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func TestYtdlDetail_CarriesEverythingKnown(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")
	_ = st.SetYtdlCompanion(id, true, "en")

	got, code := ytdlDetail(t, h, admin, id)
	if code != http.StatusOK {
		t.Fatalf("detail = %d", code)
	}
	if got.RequestID != id || got.Mode != "music" || got.Scope != "single" || got.Kind != "single" {
		t.Fatalf("detail missing basics: %+v", got)
	}
	if got.State != "downloading" {
		t.Errorf("state = %q, want the live job's state", got.State)
	}
	if got.HasLyrics == nil || !*got.HasLyrics || got.LyricsLang != "en" {
		t.Errorf("lyrics = (%v, %q), want them reported", got.HasLyrics, got.LyricsLang)
	}
	if got.CreatedAt == 0 {
		t.Error("submittedAt missing, so the detail view has no creation time to show")
	}
	// FR-033: not finished means the field is ABSENT, not 0 (which is 1970).
	if got.FinishedAt != nil {
		t.Errorf("finishedAt = %v on an unfinished download, want it absent", *got.FinishedAt)
	}
}

func TestYtdlDetail_FinishedCarriesBothTimestamps(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")
	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	if _, code := ytdlDetail(t, h, admin, id); code != http.StatusOK {
		t.Fatalf("detail = %d", code)
	}

	got, _ := ytdlDetail(t, h, admin, id)
	if got.State != "completed" {
		t.Fatalf("state = %q, want completed", got.State)
	}
	if got.FinishedAt == nil || *got.FinishedAt == 0 {
		t.Fatal("a finished download must carry when it finished")
	}
	if got.CreatedAt == 0 || *got.FinishedAt < got.CreatedAt {
		t.Fatalf("finished (%v) before created (%d)", got.FinishedAt, got.CreatedAt)
	}
	// A finished download has no progress to report.
	if got.Progress != nil {
		t.Errorf("progress = %v on a finished download, want it absent", *got.Progress)
	}
}

func TestYtdlDetail_AnotherUsersIsNotFound(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	_, theirs := ytdlDetail(t, h, bo, id)
	_, imaginary := ytdlDetail(t, h, bo, "no-such-id")
	if theirs != http.StatusNotFound {
		t.Errorf("another user's detail = %d, want 404 (403 would confirm it exists)", theirs)
	}
	if theirs != imaginary {
		t.Errorf("not-yours answers %d but not-there answers %d — the two must match", theirs, imaginary)
	}
}

func TestYtdlDetail_SubmitterIsAdminOnly(t *testing.T) {
	jobs := &fakeJobs{}
	h, _ := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")

	rec := submit(t, h, bo, `{"url":"https://youtu.be/boSong","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)

	own, _ := ytdlDetail(t, h, bo, sub.RequestID)
	if own.SubmitBy != "" {
		t.Errorf("non-admin sees submittedBy = %q, want it withheld", own.SubmitBy)
	}
	asAdmin, _ := ytdlDetail(t, h, admin, sub.RequestID)
	if asAdmin.SubmitBy == "" {
		t.Error("admin should see who submitted it")
	}
}

// FR-032. Whatever went wrong, what reaches the user is plain language — never
// a command line, a path, or raw worker output.
func TestYtdlDetail_FailureReasonIsPlainLanguage(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")
	jobs.setStatus(t, id, k8s.JobStatus{
		Conditions: []k8s.JobCondition{{Type: "Failed", Status: "True", Reason: "DeadlineExceeded"}},
	})

	got, _ := ytdlDetail(t, h, admin, id)
	if got.State != "failed" || got.Reason == "" {
		t.Fatalf("want a failed download with a reason, got %+v", got)
	}
	for _, forbidden := range []string{"yt-dlp", "/out", "--", "/v1", "exec", "Traceback"} {
		if contains2(got.Reason, forbidden) {
			t.Errorf("reason %q leaks %q — it must be plain language only", got.Reason, forbidden)
		}
	}
}

// The artwork proxy sits at /v1/ytdl/thumb, and detail at /v1/ytdl/{requestId}.
// Those overlap; this pins which one wins, because getting it wrong would break
// every thumbnail in the app in a way no other test would notice.
func TestYtdlRoutes_ThumbIsNotSwallowedByDetail(t *testing.T) {
	h, _ := newYtdlRouter(t, &fakeJobs{}, ytdlCfg())
	_ = adminAfterSetup(t, h)

	// A bad target reaches the THUMB handler (400), not the detail handler,
	// which would answer 404 for a download called "thumb".
	rec := do(t, h, "GET", "/v1/ytdl/thumb?u=https%3A%2F%2Fevil.example%2Fx.jpg", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("/v1/ytdl/thumb = %d, want the artwork handler's 400 — detail would answer 404", rec.Code)
	}
}

func TestYtdlItems_ReturnsAGroupsContents(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	owner := adminUserID(t, st)
	group := store.YtdlDownload{
		RequestID: "grp", Kind: store.YtdlKindGroup, UserID: &owner,
		SourceURL: "https://www.youtube.com/@lofi/videos", Mode: "music", Scope: "channel",
		State: "downloading", Title: "Lo-fi Beats",
	}
	if err := st.CreateYtdlDownload(group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	for i, id := range []string{"i1", "i2", "i3"} {
		item := store.YtdlDownload{
			RequestID: id, Kind: store.YtdlKindItem, ParentID: "grp", UserID: &owner,
			SourceURL: "https://youtu.be/" + id, VideoID: id, Mode: "music", Scope: "single",
			State: "queued", Origin: store.YtdlOriginExpanded, GroupName: "Lo-fi Beats",
		}
		if i == 0 {
			item.State = "completed"
		}
		if err := st.CreateYtdlDownload(item); err != nil {
			t.Fatalf("create item: %v", err)
		}
	}

	rec := do(t, h, "GET", "/v1/ytdl/grp/items", "", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("items = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Group struct {
			Title  string `json:"title"`
			Counts *struct {
				Total, Completed, Failed, Remaining int
			} `json:"counts"`
		} `json:"group"`
		Items []ytdlDetailResp `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(out.Items))
	}
	if out.Group.Counts == nil || out.Group.Counts.Total != 3 || out.Group.Counts.Completed != 1 {
		t.Fatalf("group counts = %+v, want 3 total with 1 saved", out.Group.Counts)
	}

	// A group's items are NOT top-level rows (FR-019b).
	list := listYtdl(t, h, admin)
	for _, d := range list.Downloads {
		if d.RequestID == "i1" {
			t.Fatal("an expanded item appeared as a top-level row; a large channel would crowd out everything else")
		}
	}
}

func TestYtdlItems_NotAGroupIsNotFound(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	if rec := do(t, h, "GET", "/v1/ytdl/"+id+"/items", "", admin); rec.Code != http.StatusNotFound {
		t.Fatalf("items of a single download = %d, want 404", rec.Code)
	}
}

func contains2(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

// adminUserID returns the id of the account the setup wizard created.
func adminUserID(t *testing.T, st *store.Store) int64 {
	t.Helper()
	u, err := st.GetUserByUsername("admin")
	if err != nil {
		users, err2 := st.ListUsers()
		if err2 != nil || len(users) == 0 {
			t.Fatalf("no users: %v / %v", err, err2)
		}
		return users[0].ID
	}
	return u.ID
}
