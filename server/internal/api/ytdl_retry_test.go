package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

func retry(t *testing.T, h http.Handler, auth map[string]string, id string) (ytdlRetryView, int) {
	t.Helper()
	rec := do(t, h, "POST", "/v1/ytdl/"+id+"/retry", "", auth)
	var out ytdlRetryView
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

// failOne submits a download, runs it, and fails it — returning its id.
func failOne(t *testing.T, h http.Handler, jobs *fakeJobs, st *store.Store, auth map[string]string, url string) string {
	t.Helper()
	id := ytdlRunning(t, h, jobs, st, auth, url)
	jobs.setStatus(t, id, k8s.JobStatus{Failed: 1})
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	return id
}

func TestYtdlRetry_SendsAFailedDownloadBackToTheQueue(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := failOne(t, h, jobs, st, admin, "https://youtu.be/abc")

	got, code := retry(t, h, admin, id)
	if code != http.StatusAccepted {
		t.Fatalf("retry = %d", code)
	}
	if got.State != "queued" {
		t.Fatalf("state = %q, want it back in the queue", got.State)
	}
	if got.Attempts != 2 {
		t.Fatalf("attempts = %d, want the second attempt counted", got.Attempts)
	}

	// It stays ONE download in the user's history, not two (FR-029).
	list := listYtdl(t, h, admin)
	if len(list.Downloads) != 1 {
		t.Fatalf("history has %d rows after a retry, want 1", len(list.Downloads))
	}
	// The reason from the failed attempt is cleared, and so is the finish time:
	// it has not finished any more.
	rec, _ := st.GetYtdlDownload(id)
	if rec.Reason != "" || rec.FinishedAt != nil {
		t.Fatalf("stale outcome left on a retried download: %+v", rec)
	}
}

func TestYtdlRetry_ThenActuallyRunsAgain(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := failOne(t, h, jobs, st, admin, "https://youtu.be/abc")

	created := len(jobs.created)
	if _, code := retry(t, h, admin, id); code != http.StatusAccepted {
		t.Fatalf("retry failed")
	}
	ytdlAdmit(t, jobs, st)
	if len(jobs.created) != created+1 {
		t.Fatalf("a retried download did not start a new worker (%d then %d)", created, len(jobs.created))
	}
}

func TestYtdlRetry_OnlyForFailed(t *testing.T) {
	// FR-028. Retry is offered for a failed download and nothing else — a
	// completed one would re-download what is already saved.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")
	if _, code := retry(t, h, admin, id); code != http.StatusConflict {
		t.Errorf("retry of a running download = %d, want 409", code)
	}

	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	d.reconcileYtdlOnce(context.Background())
	if _, code := retry(t, h, admin, id); code != http.StatusConflict {
		t.Errorf("retry of a completed download = %d, want 409", code)
	}
}

func TestYtdlRetry_AnotherUsersIsNotFound(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	bo := ytdlSecondUser(t, h, admin, "bo")
	id := failOne(t, h, jobs, st, admin, "https://youtu.be/abc")

	_, theirs := retry(t, h, bo, id)
	_, imaginary := retry(t, h, bo, "no-such-id")
	if theirs != http.StatusNotFound {
		t.Errorf("retrying another user's download = %d, want 404", theirs)
	}
	if theirs != imaginary {
		t.Errorf("not-yours %d vs not-there %d — must be indistinguishable", theirs, imaginary)
	}
}

// FR-027, and the regression this test exists to catch: the queue re-admits by
// design and BackoffLimit is 0, so an accidental automatic retry would look
// exactly like normal operation.
func TestYtdlRetry_NothingRetriesOnItsOwn(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := failOne(t, h, jobs, st, admin, "https://youtu.be/abc")

	created := len(jobs.created)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs})
	for i := 0; i < 20; i++ {
		d.reconcileYtdlOnce(context.Background())
	}

	rec, err := st.GetYtdlDownload(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.State != "failed" {
		t.Fatalf("state = %q after 20 cycles, want it to stay failed until a person asks", rec.State)
	}
	if rec.Attempts != 1 {
		t.Fatalf("attempts = %d, want no attempt the user did not ask for", rec.Attempts)
	}
	if len(jobs.created) != created {
		t.Fatalf("workers went from %d to %d without a user retrying", created, len(jobs.created))
	}
}

func TestYtdlRetry_AGroupRequeuesOnlyItsFailedItems(t *testing.T) {
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
		t.Fatal(err)
	}
	states := map[string]string{"ok1": "completed", "ok2": "completed", "bad1": "failed", "bad2": "failed", "wait": "queued"}
	for id, state := range states {
		if err := st.CreateYtdlDownload(store.YtdlDownload{
			RequestID: id, Kind: store.YtdlKindItem, ParentID: "grp", UserID: &owner,
			SourceURL: "https://youtu.be/" + id, VideoID: id, Mode: "music", Scope: "single",
			State: state, Origin: store.YtdlOriginExpanded,
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, code := retry(t, h, admin, "grp")
	if code != http.StatusAccepted {
		t.Fatalf("group retry = %d", code)
	}
	if got.Requeued != 2 {
		t.Fatalf("requeued %d, want only the 2 that failed", got.Requeued)
	}
	for id, was := range states {
		rec, _ := st.GetYtdlDownload(id)
		switch was {
		case "failed":
			if rec.State != "queued" {
				t.Errorf("%s = %q, want it re-queued", id, rec.State)
			}
		default:
			if rec.State != was {
				t.Errorf("%s = %q, want it left at %q — retrying must not re-run what worked", id, rec.State, was)
			}
		}
	}

	// Nothing failed any more, so a second group retry has nothing to do.
	if _, code := retry(t, h, admin, "grp"); code != http.StatusConflict {
		t.Errorf("second group retry = %d, want 409", code)
	}
}
