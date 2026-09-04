package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"synodl/server/internal/store"
	"synodl/server/internal/tasktitle"
)

// attributionWindowSecs bounds how far back a task-ownership claim may be to
// match a newly-seen task, so a stale claim can't be mis-matched to an
// unrelated task with the same name much later.
const attributionWindowSecs = 30 * 60

// Task is the minimal task shape the watcher needs (decoupled from syno.Task).
// Destination/URI feed the human-readable notification title; Size is the real
// on-disk size used to backfill download history on completion.
type Task struct {
	ID          string
	Name        string
	Status      string
	Destination string
	URI         string
	Size        int64
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
	store       *store.Store
	tasks       func(ctx context.Context) ([]Task, error)
	sender      pushSender
	version     string
	interval    time.Duration
	notifyDelay time.Duration // grace before announcing a version change (see Run)
	primed      bool          // baseline poll done — new tasks now count as "added"
	now         func() time.Time

	// activeDests is the set of destinations currently being written to, refreshed
	// on every poll. Discover reads it to tell "you already have this" from "this
	// is still arriving" (spec 0008 FR-001b) — a distinction that matters because
	// the advice differs: skip it, versus wait for it.
	//
	// Published from here rather than fetched separately precisely so that
	// distinction costs no additional read of the NAS: this watcher is already
	// polling the task list for notifications.
	destMu      sync.RWMutex
	activeDests map[string]bool
}

// ActiveDestinations returns the destinations of tasks currently downloading.
// Safe for concurrent use; the returned map is a copy and may be read freely.
func (w *Watcher) ActiveDestinations() map[string]bool {
	w.destMu.RLock()
	defer w.destMu.RUnlock()
	out := make(map[string]bool, len(w.activeDests))
	for d := range w.activeDests {
		out[d] = true
	}
	return out
}

// isActive reports whether a task is still putting bytes on disk. A finished,
// errored or paused task is NOT active: its folder either holds the content
// already or never will, and in both cases "downloading" would be a lie.
func isActive(status string) bool {
	switch strings.ToLower(status) {
	case "downloading", "waiting", "extracting", "filehosting_waiting":
		return true
	}
	return false
}

// NewWatcher wires the watcher. tasks returns the current NAS task list; a
// non-nil error (NAS unreachable / needs re-auth) just skips that round.
func NewWatcher(st *store.Store, tasks func(context.Context) ([]Task, error), sender pushSender, version string, interval time.Duration) *Watcher {
	return &Watcher{store: st, tasks: tasks, sender: sender, version: version, interval: interval, now: time.Now}
}

// SetUpdateNotifyDelay holds back the "app updated" push for d after startup, so
// a rolling deploy's replacement pod is actually serving before clients are told
// a new version is available. Without it, the push fired the instant the new
// process started and tapping it landed on a not-yet-ready backend.
func (w *Watcher) SetUpdateNotifyDelay(d time.Duration) { w.notifyDelay = d }

// Run polls until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	// Prime the baseline immediately so existing tasks aren't announced as newly
	// "added"; only tasks first seen AFTER this count as added.
	w.poll(ctx)
	w.primed = true
	// Announce a version change only after the startup grace, off the poll loop,
	// so clients aren't told to reload before this instance can serve them.
	go w.announceUpdateAfterGrace(ctx)
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

// announceUpdateAfterGrace waits out the startup grace (interruptible by ctx),
// then fires the one-off version-changed notice.
func (w *Watcher) announceUpdateAfterGrace(ctx context.Context) {
	if w.notifyDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.notifyDelay):
		}
	}
	w.maybeNotifyUpdate(ctx)
}

func (w *Watcher) poll(ctx context.Context) {
	tasks, err := w.tasks(ctx)
	if err != nil {
		return // NAS unreachable or needs re-auth — try again next tick
	}
	// Refresh the destinations Discover reads. Rebuilt wholesale so a task that
	// finished or was removed stops being reported as in progress.
	dests := make(map[string]bool)
	for _, t := range tasks {
		if d := strings.Trim(strings.TrimSpace(t.Destination), "/"); d != "" && isActive(t.Status) {
			dests[d] = true
		}
	}
	w.destMu.Lock()
	w.activeDests = dests
	w.destMu.Unlock()
	for _, t := range tasks {
		last, notified, found, err := w.store.GetWatched(t.ID)
		if err != nil {
			continue
		}
		// Human-readable body (clean title + episode marker) instead of the raw
		// release-scene file name.
		body := tasktitle.Display(t.Name, t.Destination, t.URI)
		switch {
		case !found:
			// First sight: attribute the task to whoever created it (a pending
			// claim recorded at create time), then baseline. A task already
			// finished when first seen is marked notified so we never push a stale
			// completion.
			_ = w.store.UpsertWatched(t.ID, t.Status, t.Status == "finished")
			w.attribute(t) // stamp owner from a pending name claim (direct adds)
			owner := w.ownerOf(t)
			// Only announce as "added" once we're past the startup baseline and the
			// task is genuinely in flight (not one we discovered already done).
			if w.primed && t.Status != "finished" && t.Status != "error" {
				w.notifyEvent(ctx, "added", owner, t.ID, "Download added", body)
			}
			// If it was already finished on first sight, still backfill its size.
			if t.Status == "finished" {
				w.backfillSize(t)
			}
		case t.Status == "finished" && last != "finished" && !notified:
			owner := w.ownerOf(t)
			w.notifyEvent(ctx, "completed", owner, t.ID, "Download complete", body)
			w.backfillSize(t)
			_ = w.store.UpsertWatched(t.ID, t.Status, true)
		case t.Status == "error" && last != "error":
			owner := w.ownerOf(t)
			w.notifyEvent(ctx, "failed", owner, t.ID, "Download failed", body)
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

// ownerOf resolves the SynoDL user who added a task: the stamped watched owner
// (a matched name claim, used by direct adds), else the catalog owner attributed
// by destination folder (a Discover send — its claim is by folder name and can't
// match the file-named task, so destination is the reliable source). Returns 0
// when unattributable (e.g. a task created outside SynoDL).
func (w *Watcher) ownerOf(t Task) int64 {
	if owner, ok, _ := w.store.GetWatchedOwner(t.ID); ok && owner != 0 {
		return owner
	}
	if m, err := w.store.SourceDownloads(); err == nil {
		if d, ok := m[strings.Trim(strings.TrimSpace(t.Destination), "/")]; ok && d.OwnerID != 0 {
			return d.OwnerID
		}
	}
	return 0
}

// backfillSize records the task's real on-disk size onto its download-history
// row (matched by destination + name). A no-match is benign — the download
// simply isn't tracked here, or a size was already recorded.
func (w *Watcher) backfillSize(t Task) {
	if _, err := w.store.CompleteDownloadHistory(t.Destination, t.Name, t.Size, w.now().Unix()); err != nil {
		slog.Warn("history size backfill failed", "err", err)
	}
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
	// Resolve the owner's username once, for the attribution suffix shown only to
	// all-scope (admin/owner) subscribers about someone else's download.
	ownerName := ""
	if ownerUserID != 0 {
		if u, e := w.store.GetUserByID(ownerUserID); e == nil {
			ownerName = u.Username
		}
	}
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
		// An all-scope subscriber seeing SOMEONE ELSE's download also gets who
		// added it; a subscriber is never told "added by <themselves>", and a
		// non-admin ("own" scope) never reaches here for another user's task.
		out := msg
		if scope == "any" && ownerName != "" && sub.UserID != ownerUserID {
			out = payload(title, body+" · added by "+ownerName, taskID)
		}
		w.send(ctx, sub, out)
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
