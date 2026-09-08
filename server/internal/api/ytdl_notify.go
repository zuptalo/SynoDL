package api

import (
	"context"
	"fmt"

	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// Telling someone their download finished (spec 0013, FR-025a – FR-025d).
//
// The rule that matters here is the group one. A directly submitted link
// notifies as one download, like any NAS task. An expanded playlist or channel
// notifies ONCE, when every item is final — because expansion has no ceiling,
// and notifying per item would turn one paste into several hundred pushes.
//
// Nothing else about notifications changes: the user's existing preferences
// decide whether they hear anything at all, and their existing scope decides
// whose downloads they hear about.

// notifyFinished announces a download that has just reached a final state.
//
// Called from the same place the outcome is recorded, so a download is announced
// exactly when it becomes true rather than when somebody happens to look.
func (d Deps) notifyFinished(ctx context.Context, rec store.YtdlDownload, state string) {
	if d.Notifier == nil {
		return
	}
	// An ITEM never notifies on its own (FR-025c). Its group speaks for it, once.
	if rec.Kind == store.YtdlKindItem {
		return
	}
	var owner int64
	if rec.UserID != nil {
		owner = *rec.UserID
	}

	event := "completed"
	if state == string(ytdl.StateFailed) {
		event = "failed"
	}

	title := rec.Title
	if title == "" {
		title = "YouTube download"
	}
	d.Notifier.NotifyDownload(ctx, event, owner, rec.RequestID, title, d.notifyBody(rec, state))
}

// notifyBody is the one line the notification carries.
//
// A group summarises rather than naming an item: "338 saved, 2 failed" is what
// somebody who set a channel going actually wants to know, and no single item's
// name would answer it.
func (d Deps) notifyBody(rec store.YtdlDownload, state string) string {
	if rec.Kind == store.YtdlKindGroup {
		c, err := d.Store.YtdlCounts(rec.RequestID)
		if err != nil {
			return "finished"
		}
		if c.Failed == 0 {
			return fmt.Sprintf("%d saved", c.Completed)
		}
		return fmt.Sprintf("%d saved, %d failed", c.Completed, c.Failed)
	}
	if state == string(ytdl.StateFailed) {
		if rec.Reason != "" {
			// Already plain language by the time it reaches a record (FR-032).
			return rec.Reason
		}
		return "the download did not complete"
	}
	if rec.Mode == string(ytdl.ModeMusicVideo) {
		return "saved to your music videos"
	}
	return "saved to your music"
}
