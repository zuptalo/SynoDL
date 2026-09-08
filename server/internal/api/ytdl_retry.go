package api

import (
	"net/http"

	"synodl/server/internal/httpx"
	"synodl/server/internal/store"
	"synodl/server/internal/ytdl"
)

// Retrying a failed download (spec 0013, US5).
//
// Spec 0012 forbade AUTOMATIC retry and gave the reason: a retry would repeat a
// partially completed download rather than resume it, so the worker's
// BackoffLimit is 0 and nothing in the server re-runs anything on its own. That
// still holds — this endpoint is the only route back out of a final state, and
// it exists only because a person asked.
//
// Without it, recovering from a transient failure means finding the link again
// and re-pasting it, which is exactly the manual routine the whole feature
// exists to remove.

type ytdlRetryView struct {
	RequestID string `json:"requestId"`
	State     string `json:"state"`
	Attempts  int    `json:"attempts"`
	// Requeued is how many items a GROUP retry sent back. Absent for a single
	// download, where it would only ever be 1.
	Requeued int `json:"requeued,omitempty"`
}

// handleYtdlRetry sends a failed download back to the queue.
func handleYtdlRetry(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		rec, ok := d.ytdlRecordFor(w, r, u)
		if !ok {
			return
		}

		// Retrying a GROUP means retrying the items that failed, not the group
		// itself: re-running the whole channel would re-queue everything that
		// already worked, and the group has no worker of its own to repeat.
		if rec.Kind == store.YtdlKindGroup {
			n, err := d.retryGroupItems(rec.RequestID)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "server")
				return
			}
			if n == 0 {
				httpx.Error(w, http.StatusConflict, "nothing in that download has failed")
				return
			}
			httpx.JSON(w, http.StatusAccepted, ytdlRetryView{
				RequestID: rec.RequestID,
				State:     string(ytdl.StateQueued),
				Requeued:  n,
			})
			return
		}

		// Only a failed download may be retried (FR-028). The store makes the
		// check part of the UPDATE, so two taps in quick succession cannot count
		// as two attempts.
		requeued, err := d.Store.RequeueYtdl(rec.RequestID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		if !requeued {
			httpx.Error(w, http.StatusConflict, "that download has not failed")
			return
		}

		// The old job is swept so the new attempt can take its name. A finished
		// job is safe to remove; nothing is running to strand.
		if d.Jobs != nil {
			_ = d.Jobs.DeleteJob(r.Context(), ytdl.JobName(rec.RequestID))
		}

		after, err := d.Store.GetYtdlDownload(rec.RequestID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "server")
			return
		}
		httpx.JSON(w, http.StatusAccepted, ytdlRetryView{
			RequestID: after.RequestID,
			State:     after.State,
			Attempts:  after.Attempts,
		})
	})
}

// retryGroupItems re-queues every failed item of a group, and reports how many.
func (d Deps) retryGroupItems(parentID string) (int, error) {
	var n int
	cursor := ""
	for {
		items, next, err := d.Store.ListYtdlItems(parentID, cursor, 200)
		if err != nil {
			return n, err
		}
		for _, it := range items {
			if it.State != string(ytdl.StateFailed) {
				continue
			}
			ok, err := d.Store.RequeueYtdl(it.RequestID)
			if err != nil {
				return n, err
			}
			if ok {
				n++
			}
		}
		if next == "" {
			return n, nil
		}
		cursor = next
	}
}
