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

func newWatcherStore(t *testing.T) *store.Store {
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
	return st
}

func TestWatcherNotifiesOnCompletionOnce(t *testing.T) {
	st := newWatcherStore(t)
	fs := &fakeSender{}
	tasks := []Task{{ID: "t1", Name: "ubuntu.iso", Status: "downloading"}}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return tasks, nil }, fs, "v1", time.Hour)

	w.poll(context.Background()) // baseline: downloading → no push
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

func TestWatcherIgnoresAlreadyFinished(t *testing.T) {
	st := newWatcherStore(t)
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
	st := newWatcherStore(t)
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

func TestWatcherAppUpdatePush(t *testing.T) {
	st := newWatcherStore(t)
	fs := &fakeSender{}
	w := NewWatcher(st, func(context.Context) ([]Task, error) { return nil, nil }, fs, "v2", time.Hour)

	// First run records the version silently.
	_ = st.SetLastVersionNotified("")
	w.maybeNotifyUpdate(context.Background())
	if len(fs.sent) != 0 {
		t.Fatalf("no push on first-run version record: %v", fs.sent)
	}
	// A later version change pushes once.
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
