package api

import (
	"context"
	"strings"
	"sync"
	"testing"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// fakeNotifier records what would have been announced, so notification
// behaviour can be tested without a push endpoint anywhere in sight.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []notice
}

type notice struct{ event, id, title, body string }

func (f *fakeNotifier) NotifyDownload(_ context.Context, event string, _ int64, id, title, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, notice{event, id, title, body})
}

func (f *fakeNotifier) all() []notice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]notice{}, f.sent...)
}

func TestNotify_ASingleDownloadAnnouncesItself(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	n := &fakeNotifier{}
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs, Notifier: n})
	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())

	got := n.all()
	if len(got) != 1 {
		t.Fatalf("sent %d notices, want 1: %+v", len(got), got)
	}
	if got[0].event != "completed" || got[0].id != id {
		t.Fatalf("notice = %+v", got[0])
	}
	if !strings.Contains(got[0].body, "music") {
		t.Errorf("body = %q, want it to say where the download went", got[0].body)
	}

	// And it is announced ONCE, however many cycles run afterwards.
	for i := 0; i < 5; i++ {
		d.reconcileYtdlOnce(context.Background())
	}
	if len(n.all()) != 1 {
		t.Fatalf("re-observing announced it again: %+v", n.all())
	}
}

func TestNotify_AFailureSaysWhy(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	n := &fakeNotifier{}
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs, Notifier: n})
	jobs.setStatus(t, id, k8s.JobStatus{
		Conditions: []k8s.JobCondition{{Type: "Failed", Status: "True", Reason: "DeadlineExceeded"}},
	})
	d.reconcileYtdlOnce(context.Background())

	got := n.all()
	if len(got) != 1 || got[0].event != "failed" {
		t.Fatalf("notices = %+v, want one failure", got)
	}
	if got[0].body == "" {
		t.Error("a failure notice must say something")
	}
	// FR-032: plain language, never internals — a notification is the one place
	// a reason reaches someone who is not even looking at the app.
	for _, forbidden := range []string{"yt-dlp", "/out", "--", "Traceback"} {
		if strings.Contains(got[0].body, forbidden) {
			t.Errorf("notice body %q leaks %q", got[0].body, forbidden)
		}
	}
}

// FR-025c, and the reason the group rule exists at all: with no ceiling on
// expansion, notifying per item turns one paste into several hundred pushes.
func TestNotify_AGroupAnnouncesOnceForTheWholeThing(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	n := &fakeNotifier{}
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs, Notifier: n})
	d.reconcileYtdlOnce(context.Background())

	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats",
		"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	if len(items) != 4 {
		t.Fatalf("want 4 items, got %d", len(items))
	}
	d.reconcileYtdlOnce(context.Background())
	for i, it := range items {
		if i == 0 {
			jobs.setStatus(t, it.RequestID, k8s.JobStatus{Failed: 1})
			continue
		}
		jobs.setStatus(t, it.RequestID, k8s.JobStatus{Succeeded: 1})
	}
	// Several cycles, so every item settles and the group follows.
	for i := 0; i < 4; i++ {
		d.reconcileYtdlOnce(context.Background())
	}

	got := n.all()
	if len(got) != 1 {
		t.Fatalf("sent %d notices for a 4-item group, want exactly 1: %+v", len(got), got)
	}
	if got[0].id != gid {
		t.Fatalf("notice was for %q, want the group %q", got[0].id, gid)
	}
	// The body summarises: no single item's name would answer "how did my
	// channel go?".
	if !strings.Contains(got[0].body, "3 saved") || !strings.Contains(got[0].body, "1 failed") {
		t.Fatalf("body = %q, want a summary of what happened", got[0].body)
	}
	// Something failed, so the group did not entirely work.
	if got[0].event != "failed" {
		t.Fatalf("event = %q, want failed when an item failed", got[0].event)
	}
}

func TestNotify_AGroupWithAnItemStuckSaysNothingYet(t *testing.T) {
	// FR-025c + FR-013d: the group notice fires only once EVERY item is final,
	// so a group with something still running must stay quiet — and FR-013d is
	// what guarantees it eventually will not be stuck.
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	gid := submitGroup(t, h, admin, "https://www.youtube.com/@lofi")

	n := &fakeNotifier{}
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs, Notifier: n})
	d.reconcileYtdlOnce(context.Background())
	expandName := ytdl.ExpandJobName(gid)
	jobs.setStatusByName(t, expandName, k8s.JobStatus{Succeeded: 1})
	jobs.attachPodFor(expandName, gid, ytdl.JobKindExpand)
	jobs.logs[expandName+"-worker"] = expansionOutput("Lo-fi Beats", "aaaaaaaaaaa", "bbbbbbbbbbb")
	d.reconcileYtdlOnce(context.Background())

	items, _, _ := st.ListYtdlItems(gid, "", 100)
	d.reconcileYtdlOnce(context.Background())
	jobs.setStatus(t, items[0].RequestID, k8s.JobStatus{Succeeded: 1})
	// The second one never finishes.
	for i := 0; i < 4; i++ {
		d.reconcileYtdlOnce(context.Background())
	}

	if got := n.all(); len(got) != 0 {
		t.Fatalf("announced %+v while an item was still running", got)
	}
}

func TestNotify_AnItemNeverAnnouncesItself(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	_ = adminAfterSetup(t, h)
	owner := adminUserID(t, st)

	if err := st.CreateYtdlDownload(store.YtdlDownload{
		RequestID: "grp", Kind: store.YtdlKindGroup, UserID: &owner,
		SourceURL: "https://www.youtube.com/@lofi/videos", Mode: "music", Scope: "channel",
		State: "downloading", Title: "Lo-fi Beats",
	}); err != nil {
		t.Fatal(err)
	}
	item := store.YtdlDownload{
		RequestID: "i1", Kind: store.YtdlKindItem, ParentID: "grp", UserID: &owner,
		SourceURL: "https://youtu.be/i1", VideoID: "i1", Mode: "music", Scope: "single",
		State: "downloading", Origin: store.YtdlOriginExpanded,
	}
	if err := st.CreateYtdlDownload(item); err != nil {
		t.Fatal(err)
	}

	n := &fakeNotifier{}
	d := InitCaches(Deps{Cfg: ytdlCfg(), Store: st, Jobs: jobs, Notifier: n})
	d.captureTerminal(item, "completed", "")

	if got := n.all(); len(got) != 0 {
		t.Fatalf("an item announced itself: %+v — its group speaks for it, once", got)
	}
}
