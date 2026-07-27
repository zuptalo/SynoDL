package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"synodl/server/internal/store"
)

// Task is the minimal task shape the watcher needs (decoupled from syno.Task).
type Task struct {
	ID     string
	Name   string
	Status string
}

// pushSender is the subset of *Sender the watcher uses (fakeable in tests).
type pushSender interface {
	Send(ctx context.Context, sub store.Subscription, payload []byte) (gone bool, err error)
}

// Watcher polls the NAS on a bounded cadence and pushes an opt-in notification
// when a download transitions to finished — so it works while every client is
// offline (the whole point). It also fires a one-off app-update notice when the
// running version changes.
type Watcher struct {
	store    *store.Store
	tasks    func(ctx context.Context) ([]Task, error)
	sender   pushSender
	version  string
	interval time.Duration
}

// NewWatcher wires the watcher. tasks returns the current NAS task list; a
// non-nil error (NAS unreachable / needs re-auth) just skips that round.
func NewWatcher(st *store.Store, tasks func(context.Context) ([]Task, error), sender pushSender, version string, interval time.Duration) *Watcher {
	return &Watcher{store: st, tasks: tasks, sender: sender, version: version, interval: interval}
}

// Run polls until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.maybeNotifyUpdate(ctx)
	// Prime the baseline immediately so a task that finishes right after startup
	// still triggers on the next tick rather than being missed.
	w.poll(ctx)
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.poll(ctx)
		}
	}
}

func (w *Watcher) poll(ctx context.Context) {
	tasks, err := w.tasks(ctx)
	if err != nil {
		return // NAS unreachable or needs re-auth — try again next tick
	}
	for _, t := range tasks {
		last, notified, found, err := w.store.GetWatched(t.ID)
		if err != nil {
			continue
		}
		switch {
		case !found:
			// Baseline on first sight. If a task is already finished when we first
			// see it (e.g. it completed before we were watching), mark it notified
			// so we never push a stale completion.
			_ = w.store.UpsertWatched(t.ID, t.Status, t.Status == "finished")
		case t.Status == "finished" && last != "finished" && !notified:
			w.notifyCompletion(ctx, t)
			_ = w.store.UpsertWatched(t.ID, t.Status, true)
		case t.Status != last:
			_ = w.store.UpsertWatched(t.ID, t.Status, notified)
		}
	}
}

func (w *Watcher) notifyCompletion(ctx context.Context, t Task) {
	w.fanOut(ctx, payload("Download complete", t.Name))
}

func (w *Watcher) maybeNotifyUpdate(ctx context.Context) {
	last, err := w.store.LastVersionNotified()
	if err != nil {
		return
	}
	if last == "" {
		// First boot: record the current version without pushing (no notice on install).
		_ = w.store.SetLastVersionNotified(w.version)
		return
	}
	if last != w.version {
		w.fanOut(ctx, payload("SynoDL updated", "A new version is available."))
		_ = w.store.SetLastVersionNotified(w.version)
	}
}

// fanOut sends payload to every opted-in device, pruning ones the push service
// reports gone.
func (w *Watcher) fanOut(ctx context.Context, body []byte) {
	subs, err := w.store.OptedInSubscriptions()
	if err != nil {
		return
	}
	for _, sub := range subs {
		gone, err := w.sender.Send(ctx, sub, body)
		if gone {
			_ = w.store.DeleteSubscription(sub.Endpoint)
			continue
		}
		if err != nil {
			slog.Warn("push send failed", "err", err)
		}
	}
}

// payload is the JSON the service worker reads to show a plain-text notification.
func payload(title, body string) []byte {
	b, _ := json.Marshal(map[string]string{"title": title, "body": body})
	return b
}
