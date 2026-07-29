package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"synodl/server/internal/store"
)

// attributionWindowSecs bounds how far back a task-ownership claim may be to
// match a newly-seen task, so a stale claim can't be mis-matched to an
// unrelated task with the same name much later.
const attributionWindowSecs = 30 * 60

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

// Watcher polls the NAS on a bounded cadence and pushes opt-in notifications for
// download events — added, completed, failed — honoring each user's preferences
// and whether they subscribe to every user's tasks or only their own (spec
// 1004). It also fires a one-off app-update notice when the running version
// changes. It works while every client is offline (the whole point).
type Watcher struct {
	store    *store.Store
	tasks    func(ctx context.Context) ([]Task, error)
	sender   pushSender
	version  string
	interval time.Duration
	primed   bool // baseline poll done — new tasks now count as "added"
	now      func() time.Time
}

// NewWatcher wires the watcher. tasks returns the current NAS task list; a
// non-nil error (NAS unreachable / needs re-auth) just skips that round.
func NewWatcher(st *store.Store, tasks func(context.Context) ([]Task, error), sender pushSender, version string, interval time.Duration) *Watcher {
	return &Watcher{store: st, tasks: tasks, sender: sender, version: version, interval: interval, now: time.Now}
}

// Run polls until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.maybeNotifyUpdate(ctx)
	// Prime the baseline immediately so existing tasks aren't announced as newly
	// "added"; only tasks first seen AFTER this count as added.
	w.poll(ctx)
	w.primed = true
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
			// First sight: attribute the task to whoever created it (a pending
			// claim recorded at create time), then baseline. A task already
			// finished when first seen is marked notified so we never push a stale
			// completion.
			_ = w.store.UpsertWatched(t.ID, t.Status, t.Status == "finished")
			owner := w.attribute(t)
			// Only announce as "added" once we're past the startup baseline and the
			// task is genuinely in flight (not one we discovered already done).
			if w.primed && t.Status != "finished" && t.Status != "error" {
				w.notifyEvent(ctx, "added", owner, t.ID, "Download added", t.Name)
			}
		case t.Status == "finished" && last != "finished" && !notified:
			owner, _, _ := w.store.GetWatchedOwner(t.ID)
			w.notifyEvent(ctx, "completed", owner, t.ID, "Download complete", t.Name)
			_ = w.store.UpsertWatched(t.ID, t.Status, true)
		case t.Status == "error" && last != "error":
			owner, _, _ := w.store.GetWatchedOwner(t.ID)
			w.notifyEvent(ctx, "failed", owner, t.ID, "Download failed", t.Name)
			_ = w.store.UpsertWatched(t.ID, t.Status, notified)
		case t.Status != last:
			_ = w.store.UpsertWatched(t.ID, t.Status, notified)
		}
	}
}

// attribute resolves and stamps the owner of a freshly-seen task from a pending
// claim; returns 0 when the task can't be attributed (owner stays unset, so
// only "any"-scope subscribers are notified about it).
func (w *Watcher) attribute(t Task) int64 {
	owner, ok, err := w.store.ClaimOwner(t.Name, w.now().Unix()-attributionWindowSecs)
	if err != nil || !ok {
		return 0
	}
	_ = w.store.SetWatchedOwner(t.ID, owner)
	return owner
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
		w.fanOut(ctx, payload("SynoDL updated", "A new version is available.", ""))
		_ = w.store.SetLastVersionNotified(w.version)
	}
}

// notifyEvent sends a task-event notification only to subscribers whose prefs
// enable that event and whose scope covers this task (any user's, or — when the
// task is attributed to them — their own).
func (w *Watcher) notifyEvent(ctx context.Context, event string, ownerUserID int64, taskID, title, body string) {
	subs, err := w.store.OptedInSubscriptions()
	if err != nil {
		return
	}
	body = truncate(body, 120)
	msg := payload(title, body, taskID)
	for _, sub := range subs {
		prefs, err := w.store.GetNotificationPrefs(sub.UserID)
		if err != nil {
			continue
		}
		if !prefEnabled(prefs, event) {
			continue
		}
		// Effective scope is role-aware: a non-admin only ever hears about their
		// own tasks; an admin defaults to everyone's (matching what they see in
		// the list) but may opt down to "own".
		isAdmin := false
		if u, e := w.store.GetUserByID(sub.UserID); e == nil {
			isAdmin = u.IsAdmin
		}
		scope, e := w.store.EffectiveNotificationScope(sub.UserID, isAdmin)
		if e != nil {
			continue
		}
		if scope != "any" && sub.UserID != ownerUserID {
			continue // "own" scope: only this user's own (attributed) tasks
		}
		w.send(ctx, sub, msg)
	}
}

func prefEnabled(p store.NotificationPrefs, event string) bool {
	switch event {
	case "added":
		return p.NotifyAdded
	case "completed":
		return p.NotifyCompleted
	case "failed":
		return p.NotifyFailed
	}
	return false
}

// fanOut sends to every opted-in device regardless of prefs — for
// instance-level notices (app updates), not per-task events.
func (w *Watcher) fanOut(ctx context.Context, body []byte) {
	subs, err := w.store.OptedInSubscriptions()
	if err != nil {
		return
	}
	for _, sub := range subs {
		w.send(ctx, sub, body)
	}
}

func (w *Watcher) send(ctx context.Context, sub store.Subscription, body []byte) {
	gone, err := w.sender.Send(ctx, sub, body)
	if gone {
		_ = w.store.DeleteSubscription(sub.Endpoint)
		return
	}
	if err != nil {
		slog.Warn("push send failed", "err", err)
	}
}

// payload is the JSON the service worker reads to show a plain-text
// notification. taskId (empty for instance-level notices like app updates) lets
// the client deep-link a tapped notification to that task's detail.
func payload(title, body, taskID string) []byte {
	m := map[string]string{"title": title, "body": body}
	if taskID != "" {
		m["taskId"] = taskID
	}
	b, _ := json.Marshal(m)
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
