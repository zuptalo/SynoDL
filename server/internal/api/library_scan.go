package api

import (
	"context"
	"sort"
	"time"
)

// The background scan that keeps the reading of what is on the NAS current
// (spec 0011).
//
// Before this existed, the reading was built only when somebody browsed, and
// only for the folders behind the titles they happened to look at. That made the
// first request after every deploy pay for it, and left anything nobody had
// browsed unknown indefinitely. The scan moves that work off the request path.

// LibraryScanInterval is how often a cycle runs.
//
// Between the watcher's 30-second task poll and the source keep-alive's 15
// minutes. A folder added to the NAS by any other means shows up within one
// cycle, which is the case this cadence is actually for — a download made
// THROUGH SynoDL does not wait for it, because finishing enqueues its folder.
const LibraryScanInterval = 10 * time.Minute

// scanFolderBudget bounds how many title folders one cycle re-reads.
//
// A folder costs a file listing plus a directory listing plus one listing per
// season, so an unbounded cycle over a library of several hundred titles would
// be a burst of thousands of calls at the NAS every interval. Bounded and
// oldest-first, the library converges over several cycles instead — which is the
// right trade, because nothing here is urgent: what IS urgent (a download that
// just landed) jumps the queue.
const scanFolderBudget = 24

// RunLibraryScan refreshes the reading on a schedule until ctx is cancelled.
//
// One cycle runs immediately, so a fresh instance has a reading without waiting
// out the first interval.
func (d Deps) RunLibraryScan(ctx context.Context, interval time.Duration) {
	d.scanOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.scanOnce(ctx)
		}
	}
}

// RefreshFolder asks for one folder to be re-read at the front of the next
// cycle. Called when a download finishes (FR-007) and when one is sent
// (FR-008), so the folder the user is actually watching does not wait behind
// the rest of the library.
//
// It only enqueues. Reading the NAS inline would put a download's completion on
// the critical path of whatever triggered it, and the whole point of the scan is
// that this work happens off to one side.
func (d Deps) RefreshFolder(folder string) {
	if d.lib == nil || folder == "" {
		return
	}
	d.lib.mu.Lock()
	if d.lib.scanQueue == nil {
		d.lib.scanQueue = map[string]bool{}
	}
	d.lib.scanQueue[folder] = true
	d.lib.mu.Unlock()
}

// scanOnce runs one cycle: refresh the parent listings, forget folders that are
// gone, then re-read a bounded number of title folders.
func (d Deps) scanOnce(ctx context.Context) {
	if d.lib == nil || d.Store == nil || d.NAS == nil {
		return
	}
	// Do nothing at all until the instance has been set up.
	//
	// The first cycle runs at boot, and on a brand-new instance that is the same
	// moment the operator is completing the setup wizard. There is nothing to
	// read — no NAS is configured yet — but the cycle still opened a write
	// transaction, and SQLite let that contend with the setup request's own
	// writes. The e2e stateful stack failed its setup with a 500 on a slower
	// machine while passing locally, which is exactly what a lock race looks like.
	if _, err := d.Store.GetOperatorConfig(); err != nil {
		return
	}
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), libraryBuildTimeout)
	defer cancel()

	// Layer one. buildLibraryIndex already persists a successful reading and
	// already declines to persist a failed one, so a NAS that is down leaves
	// everything stored exactly as it was.
	ix, ok := d.buildLibraryIndex(bctx)
	if !ok {
		return
	}
	// Publish it, so a browser arriving mid-cycle gets the fresh reading rather
	// than rebuilding it a second time.
	d.lib.mu.Lock()
	d.lib.index, d.lib.builtAt, d.lib.builtOK = ix, time.Now(), true
	d.lib.mu.Unlock()

	folders := ix.Folders()
	// A title folder that is no longer under a configured parent must stop being
	// answered for (FR-009). Pruning against an empty set is a no-op in the store,
	// which is what protects an instance whose sources have all been removed from
	// having its reading silently emptied by a half-finished scan.
	_ = d.Store.PruneLibraryEvidence(folders)
	if len(folders) == 0 {
		return
	}

	for _, folder := range d.scanBatch(folders) {
		// Reading through folderEvidence rather than around it keeps one code path
		// for "what does this folder hold", so the scan and a browsing user can
		// never disagree about the answer. It writes through to the store itself.
		if _, ok := d.folderEvidence(bctx, folder); !ok {
			// The NAS went away mid-cycle. Stop rather than spend the rest of the
			// budget on calls that will all fail — and do NOT reconcile, because a
			// half-read cycle is not evidence that anything was removed.
			return
		}
	}

	// Reconcile what we remember against what is actually there, so content the
	// user has deleted stops being remembered (spec 1029).
	//
	// AFTER the reads, not before: a folder read this cycle is judged on the
	// reading just taken rather than on last cycle's. It costs no NAS call of its
	// own — everything it needs is the index above and the stored readings. A
	// recorded folder the scan has not reached yet simply reports as unknown and
	// is left alone until it has.
	d.forgetRemovedContent(bctx, ix)
}

// scanBatch picks which folders this cycle re-reads: anything explicitly
// enqueued first, then the least-recently-read, then anything never read at all.
func (d Deps) scanBatch(folders []string) []string {
	known := make(map[string]bool, len(folders))
	for _, f := range folders {
		known[f] = true
	}

	d.lib.mu.Lock()
	queued := make([]string, 0, len(d.lib.scanQueue))
	for f := range d.lib.scanQueue {
		// A folder enqueued for a download that landed somewhere no longer
		// configured is dropped rather than read.
		if known[f] {
			queued = append(queued, f)
		}
	}
	d.lib.scanQueue = nil
	d.lib.mu.Unlock()
	sort.Strings(queued) // deterministic, so a cycle is reproducible in tests

	batch := make([]string, 0, scanFolderBudget)
	taken := map[string]bool{}
	add := func(f string) bool {
		if taken[f] || len(batch) >= scanFolderBudget {
			return len(batch) < scanFolderBudget
		}
		taken[f] = true
		batch = append(batch, f)
		return true
	}
	for _, f := range queued {
		if !add(f) {
			return batch
		}
	}

	// Folders never read at all come next, ahead of re-reading ones we already
	// have an answer for.
	//
	// The order matters more than it looks. Least-recently-read first, on its own,
	// fills every cycle's budget with folders that already have a reading — so a
	// library larger than the budget never finishes its FIRST pass, and the tail
	// stays unknown forever. Coverage first, then refresh.
	for _, f := range folders {
		if len(batch) >= scanFolderBudget {
			return batch
		}
		if taken[f] {
			continue
		}
		if _, found, err := d.Store.GetLibraryEvidence(f); err == nil && found {
			continue
		}
		add(f)
	}

	// Whatever budget is left goes on refreshing the least-recently-read, which
	// is how a folder changed on the NAS out of band is eventually noticed.
	if len(batch) >= scanFolderBudget {
		return batch
	}
	if stale, err := d.Store.StaleLibraryEvidence(scanFolderBudget); err == nil {
		for _, f := range stale {
			if known[f] && !add(f) {
				return batch
			}
		}
	}
	return batch
}
