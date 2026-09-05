package push

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"synodl/server/internal/store"
)

type fakeSender struct {
	sent []string // payloads sent
	gone bool
}

func (f *fakeSender) Send(_ context.Context, sub store.Subscription, payload []byte) (bool, error) {
	f.sent = append(f.sent, string(payload))
	return f.gone, nil
}

// newWatcherStore returns a store with one admin user + an opted-in subscription.
// An admin's effective scope defaults to "any" (everyone's tasks); tests that
// want the "own" path set it explicitly. Returns the user id so tests can
// attribute tasks / tune prefs.
func newWatcherStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_ = st.SaveOperatorConfig(store.OperatorConfig{NASAddress: "n", NASPort: 1, NASAccount: "a"})
	uid, _ := st.CreateUser("u", "h", true)
	_ = st.SaveSubscription(uid, "https://push.example/one", "p", "a", true)
	return st, uid
}

func claim(t *testing.T, st *store.Store, uid int64, name string) {
	t.Helper()
	if err := st.AddTaskClaim(uid, name, time.Now().Unix()); err != nil {
		t.Fatalf("AddTaskClaim: %v", err)
	}
}

func TestWatcherNotifiesOnCompletionOnce(t *testing.T) {
	st, uid := newWatcherStore(t)
	claim(t, st, uid, "ubuntu.iso") // the user created this task
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "ubuntu.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)

	w.poll(context.Background()) // baseline: downloading → attributes, no push
	if len(fs.sent) != 0 {
		t.Fatalf("push on baseline: %v", fs.sent)
	}
	tasks[0].Status = "finished"
	w.poll(context.Background()) // transition → one push naming the task
	if len(fs.sent) != 1 || !contains(fs.sent[0], "ubuntu.iso") {
		t.Fatalf("expected one completion push naming the task, got %v", fs.sent)
	}
	w.poll(context.Background()) // still finished → no duplicate
	if len(fs.sent) != 1 {
		t.Fatalf("duplicate push: %v", fs.sent)
	}
}

func TestWatcherOwnScopeSkipsUnattributedTask(t *testing.T) {
	st, uid := newWatcherStore(t) // no claim → task can't be attributed
	// This admin opted down to "own", so unattributed tasks must be skipped.
	_ = st.SaveNotificationPrefs(uid, store.NotificationPrefs{NotifyCompleted: true, Scope: "own"}, 0)
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "someone-elses.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())
	if len(fs.sent) != 0 {
		t.Fatalf("own-scope must not notify for an unattributed task: %v", fs.sent)
	}
}

// A non-admin's effective scope is always "own" — even the default — so they're
// never notified about a task they didn't create.
func TestWatcherNonAdminDefaultSkipsUnattributed(t *testing.T) {
	st, _ := newWatcherStore(t)
	// Drop the seeded admin's subscription (an admin defaults to "any" and would
	// legitimately be notified) so only the non-admin below is under test.
	_ = st.DeleteSubscription("https://push.example/one")
	uid, _ := st.CreateUser("regular", "h", false)
	_ = st.SaveSubscription(uid, "https://push.example/regular", "p", "a", true)
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "someone-elses.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())
	for _, p := range fs.sent {
		if contains(p, "someone-elses.iso") {
			t.Fatalf("non-admin must not be notified about others' tasks: %v", fs.sent)
		}
	}
}

// An admin with no explicit preference defaults to "any", so they hear about a
// task nobody has been attributed for (the seeded admin has default prefs).
func TestWatcherAdminDefaultNotifiesUnattributed(t *testing.T) {
	st, _ := newWatcherStore(t) // admin, no prefs row → effective scope "any"
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "unclaimed.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())
	if len(fs.sent) != 1 || !contains(fs.sent[0], "unclaimed.iso") {
		t.Fatalf("admin default (any) must be notified about an unattributed task: %v", fs.sent)
	}
}

func TestWatcherAnyScopeNotifiesUnattributedTask(t *testing.T) {
	st, uid := newWatcherStore(t)
	_ = st.SaveNotificationPrefs(uid, store.NotificationPrefs{NotifyCompleted: true, Scope: "any"}, 0)
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "anything.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())
	if len(fs.sent) != 1 {
		t.Fatalf("any-scope must notify for any task, got %v", fs.sent)
	}
}

func TestWatcherNotifiesOnFailure(t *testing.T) {
	st, uid := newWatcherStore(t)
	claim(t, st, uid, "bad.iso")
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "bad.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background()) // baseline attributes
	tasks[0].Status = "error"
	w.poll(context.Background()) // failed → push
	if len(fs.sent) != 1 || !contains(fs.sent[0], "bad.iso") {
		t.Fatalf("expected one failure push, got %v", fs.sent)
	}
	w.poll(context.Background()) // still error → no duplicate
	if len(fs.sent) != 1 {
		t.Fatalf("duplicate failure push: %v", fs.sent)
	}
}

func TestWatcherAddedOnlyWhenEnabledAndPrimed(t *testing.T) {
	st, uid := newWatcherStore(t)
	_ = st.SaveNotificationPrefs(uid,
		store.NotificationPrefs{NotifyAdded: true, NotifyCompleted: true, NotifyFailed: true, Scope: "own"}, 0)
	fs := &fakeSender{}
	var tasks []Task
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)

	w.poll(context.Background()) // baseline (empty) — not primed yet
	w.primed = true

	claim(t, st, uid, "new.iso")
	tasks = []Task{{ID: "n1", Name: "new.iso", Status: "downloading"}}
	w.poll(context.Background()) // first sight + primed → "added" push
	if len(fs.sent) != 1 || !contains(fs.sent[0], "new.iso") {
		t.Fatalf("expected one added push, got %v", fs.sent)
	}
}

func TestWatcherIgnoresAlreadyFinished(t *testing.T) {
	st, _ := newWatcherStore(t)
	fs := &fakeSender{}
	w := NewWatcher(st, func(context.Context) ([]Task, error) {
		return []Task{{ID: "old", Name: "done.iso", Status: "finished"}}, nil
	}, fs, "v1", time.Hour)
	w.poll(context.Background())
	w.poll(context.Background())
	if len(fs.sent) != 0 {
		t.Fatalf("should not push for a task already finished on first sight: %v", fs.sent)
	}
}

func TestWatcherPrunesGoneSubscription(t *testing.T) {
	st, uid := newWatcherStore(t)
	claim(t, st, uid, "x")
	fs := &fakeSender{gone: true}
	tasks := []Task{{ID: "t1", Name: "x", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())
	subs, _ := st.OptedInSubscriptions()
	if len(subs) != 0 {
		t.Fatalf("gone subscription should have been pruned, still have %d", len(subs))
	}
}

// The version-changed push is held back until the startup grace elapses; if the
// instance is cancelled during the grace (a deploy that never becomes ready), no
// premature notice fires.
func TestWatcherUpdatePushWaitsForGrace(t *testing.T) {
	st, _ := newWatcherStore(t)
	_ = st.SetLastVersionNotified("v1")
	fs := &fakeSender{}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return nil, nil }, fs, "v2", time.Hour)
	w.SetUpdateNotifyDelay(time.Hour) // effectively "not yet"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // instance goes away before the grace elapses
	w.announceUpdateAfterGrace(ctx)
	if len(fs.sent) != 0 {
		t.Fatalf("no push should fire before the grace elapses: %v", fs.sent)
	}

	// With no delay it fires immediately.
	w.SetUpdateNotifyDelay(0)
	w.announceUpdateAfterGrace(context.Background())
	if len(fs.sent) != 1 || !contains(fs.sent[0], "new version") {
		t.Fatalf("expected the update push once the grace is clear, got %v", fs.sent)
	}
}

func TestWatcherAppUpdatePush(t *testing.T) {
	st, _ := newWatcherStore(t)
	fs := &fakeSender{}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return nil, nil }, fs, "v2", time.Hour)

	_ = st.SetLastVersionNotified("")
	w.maybeNotifyUpdate(context.Background())
	if len(fs.sent) != 0 {
		t.Fatalf("no push on first-run version record: %v", fs.sent)
	}
	_ = st.SetLastVersionNotified("v1")
	w.maybeNotifyUpdate(context.Background())
	if len(fs.sent) != 1 || !contains(fs.sent[0], "new version") {
		t.Fatalf("expected one app-update push, got %v", fs.sent)
	}
	last, _ := st.LastVersionNotified()
	if last != "v2" {
		t.Fatalf("version HWM = %q, want v2", last)
	}
}

// A catalog download owned by a regular user is attributed by destination. The
// all-scope admin is notified with a readable title AND "added by <user>"; the
// owner (own scope) gets the same readable title but is never told they added it.
func TestWatcherReadableTitleAndAttribution(t *testing.T) {
	st, _ := newWatcherStore(t) // seeded admin (any scope) subscribed at /one
	alice, _ := st.CreateUser("alice", "h", false)
	_ = st.SaveSubscription(alice, "https://push.example/alice", "p", "a", true)
	// Catalog send attributed to alice by its destination folder.
	_ = st.SaveSourceDownload(store.SourceDownload{
		Destination: "tv/Rick and Morty", MediaType: "series", Title: "Rick and Morty", OwnerID: alice,
	}, time.Now().Unix())

	fs := &fakeSender{}
	tasks := []Task{{
		ID: "t1", Name: "Rick.and.Morty.S01E05.1080p.WEB-DL.mkv",
		Status: "downloading", Destination: "tv/Rick and Morty",
	}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())

	if len(fs.sent) != 2 {
		t.Fatalf("want 2 completion pushes (admin + owner), got %d: %v", len(fs.sent), fs.sent)
	}
	var adminMsg, ownerMsg string
	for _, m := range fs.sent {
		if contains(m, "added by alice") {
			adminMsg = m
		} else {
			ownerMsg = m
		}
	}
	if adminMsg == "" || !contains(adminMsg, "Rick and Morty · S01E05") {
		t.Fatalf("admin push should have readable title + attribution: %v", fs.sent)
	}
	if ownerMsg == "" || !contains(ownerMsg, "Rick and Morty · S01E05") || contains(ownerMsg, "added by") {
		t.Fatalf("owner push should have readable title and NO attribution: %v", fs.sent)
	}
	// The raw release-scene name must not appear in either body.
	for _, m := range fs.sent {
		if contains(m, "1080p") {
			t.Fatalf("raw file name leaked into notification: %q", m)
		}
	}
}

// On completion, the task's real size is backfilled onto its history row.
func TestWatcherBackfillsSizeOnCompletion(t *testing.T) {
	st, uid := newWatcherStore(t)
	_ = st.AddDownloadHistory(store.DownloadHistory{
		UserID: uid, Source: store.SourceCatalog, Category: store.CategoryMovie,
		Destination: "movies/Dune", TaskName: "Dune.mkv", CreatedAt: 1000,
	})
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "Dune.mkv", Status: "downloading", Destination: "movies/Dune", Size: 4096}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)
	w.poll(context.Background())
	tasks[0].Status = "finished"
	w.poll(context.Background())

	sum, _ := st.StatsSummary([]int64{uid})
	if len(sum) != 1 || sum[0].AvgSize[store.CategoryMovie] != 4096 {
		t.Fatalf("size not backfilled: %+v", sum)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Spec 0011: a download that finishes must cause its folder to be re-read, so
// the title reflects it without the user having to browse to it. The watcher is
// already the thing that sees the finish, so it is the thing that says so.
func TestTheFinishCallbackFiresOnceWithTheDestination(t *testing.T) {
	st, _ := newWatcherStore(t)
	var got []string
	tasks := []Task{{ID: "t1", Name: "Dune.2021.mkv", Status: "downloading", Destination: "movie/Dune 2021"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil },
		&fakeSender{}, "1.0.0", time.Minute)
	w.OnFinished = func(dest string) { got = append(got, dest) }

	w.poll(context.Background()) // baseline: still downloading
	if len(got) != 0 {
		t.Fatalf("a task in flight must not report as finished: %v", got)
	}

	tasks[0].Status = "finished"
	w.poll(context.Background())
	if len(got) != 1 || got[0] != "movie/Dune 2021" {
		t.Fatalf("the finish must report its destination once, got %v", got)
	}

	// Polling again must not report it a second time — a re-read per poll for the
	// life of a finished task would be a NAS call every 30 seconds, forever.
	w.poll(context.Background())
	if len(got) != 1 {
		t.Fatalf("a finish must be reported once, got %v", got)
	}
}

// Spec 1029: deciding whether content has been REMOVED from the NAS asks a
// different question from Discover's "is this arriving" badge. A paused task's
// folder is empty right now and will not be when it resumes, so it must not read
// as a removal — and a paused task is deliberately NOT "active".
func TestUnfinishedDestinationsIncludePausedAndErrored(t *testing.T) {
	st, _ := newWatcherStore(t)
	tasks := []Task{
		{ID: "a", Name: "a.mkv", Status: "downloading", Destination: "movie/Downloading"},
		{ID: "b", Name: "b.mkv", Status: "paused", Destination: "movie/Paused"},
		{ID: "c", Name: "c.mkv", Status: "error", Destination: "movie/Errored"},
		{ID: "d", Name: "d.mkv", Status: "waiting", Destination: "movie/Waiting"},
		{ID: "e", Name: "e.mkv", Status: "finished", Destination: "movie/Finished"},
	}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil },
		&fakeSender{}, "1.0.0", time.Minute)
	w.poll(context.Background())

	pending := w.UnfinishedDestinations()
	for _, want := range []string{"movie/Downloading", "movie/Paused", "movie/Errored", "movie/Waiting"} {
		if !pending[want] {
			t.Errorf("%s is unfinished; its folder may legitimately be empty", want)
		}
	}
	if pending["movie/Finished"] {
		t.Error("a finished task's folder should hold its content; it must not be protected from cleanup")
	}

	// The narrower set is unchanged: a paused download is not "arriving".
	active := w.ActiveDestinations()
	if active["movie/Paused"] || active["movie/Errored"] {
		t.Error("paused/errored must stay out of the Discover downloading badge")
	}
	if !active["movie/Downloading"] {
		t.Error("a downloading task should still read as active")
	}
}
