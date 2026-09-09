package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/store"
)

// What a watcher is told, and when (spec 1038).
//
// These tests drive whole reconcile cycles rather than calling watchYtdl
// directly, because WHERE the diff runs is the design: two of the four things
// that make up a download's shown state are only known inside a cycle, and a
// test that skipped the cycle would prove nothing about either.

// watchOn subscribes to a Deps' hub and returns everything published to it.
func watchOn(t *testing.T, d Deps, userID int64, isAdmin bool) func() []ytdlChange {
	t.Helper()
	ch, cancel := d.ytdlHub.subscribe(userID, isAdmin)
	t.Cleanup(cancel)
	return func() []ytdlChange {
		var out []ytdlChange
		for {
			select {
			case b, ok := <-ch:
				if !ok {
					return out
				}
				out = append(out, b...)
			default:
				return out
			}
		}
	}
}

func changeFor(cs []ytdlChange, id string) (ytdlChange, bool) {
	for _, c := range cs {
		if c.RequestID == id {
			return c, true
		}
	}
	return ytdlChange{}, false
}

// stateIn reads the state out of a rendered change, which is the only place it
// exists by the time it reaches a reader.
func stateIn(t *testing.T, c ytdlChange) string {
	t.Helper()
	var v struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(c.Plain, &v); err != nil {
		t.Fatalf("unmarshal change: %v", err)
	}
	return v.State
}

// A restart must not reorder anybody's list. The first publish of a process has
// nothing to compare against, so it carries the whole unfinished set — as
// CHANGED, never created: a client merges a change it recognises and ignores one
// it does not, whereas `created` means "put this at the top".
func TestWatch_TheFirstPublishNeverClaimsAnythingIsNew(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, 1, true)

	d.reconcileYtdlOnce(context.Background())
	first := drain()
	if len(first) == 0 {
		t.Fatal("the first cycle published nothing; a client that connected before it would never hear about what is already running")
	}
	for _, c := range first {
		if c.Created {
			t.Fatalf("%s was announced as new on a first publish; a restart would reorder every watching list", c.RequestID)
		}
	}

	// And having spoken once, an unchanged download stays quiet.
	d.reconcileYtdlOnce(context.Background())
	if again := drain(); len(again) != 0 {
		t.Fatalf("unchanged downloads were republished: %v", ids(again))
	}
}

// The regression this design got wrong first time round.
//
// The fingerprints used to be CLEARED whenever nobody was watching, and the
// first cycle after somebody connected only re-seeded them. Anything that
// happened between that client fetching its list and that seed was therefore
// never announced — it matched from the next cycle on, so it stayed missing
// until something else about the same download moved. In practice: a progress
// reading that landed in exactly that window, and a bar that never appeared.
func TestWatch_AChangeMadeBeforeTheFirstWatchingCycleIsStillAnnounced(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	d.reconcileYtdlOnce(context.Background()) // cycles run with nobody watching

	// Somebody connects, and the download moves before the next cycle.
	drain := watchOn(t, d, 1, true)
	jobs.emitFor(id, "[synodl] status=downloading downloaded=55 total=100")

	d.reconcileYtdlOnce(context.Background())

	c, ok := changeFor(drain(), id)
	if !ok {
		t.Fatal("the change was swallowed by the first cycle after connecting")
	}
	var v struct {
		Progress *float64 `json:"progress"`
	}
	_ = json.Unmarshal(c.Plain, &v)
	if v.Progress == nil {
		t.Fatal("the announcement carried no progress, so the bar would never appear")
	}
}

// FR-004: only what changed, and nothing else.
func TestWatch_PublishesOnlyWhatChanged(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	a := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/aaa")
	b := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/bbb")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, 1, true)
	d.reconcileYtdlOnce(context.Background()) // seed
	drain()

	// One of them finishes.
	jobs.setStatus(t, a, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())

	got := drain()
	if _, ok := changeFor(got, a); !ok {
		t.Fatalf("the download that changed was not published; got %v", ids(got))
	}
	if _, ok := changeFor(got, b); ok {
		t.Fatalf("a download that did not change was published; got %v", ids(got))
	}
}

// The load-bearing case. Progress lives in an in-memory cache that nothing ever
// writes to the store, so a diff over stored rows would never see it move —
// which is the whole reason the watch step runs inside the cycle.
func TestWatch_AProgressReadingIsAChange(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, 1, true)
	d.reconcileYtdlOnce(context.Background())
	drain()

	jobs.emitFor(id, "[synodl] status=downloading downloaded=42 total=100")
	d.reconcileYtdlOnce(context.Background())

	c, ok := changeFor(drain(), id)
	if !ok {
		t.Fatal("a progress reading moved and nobody was told")
	}
	var v struct {
		Progress *float64 `json:"progress"`
	}
	_ = json.Unmarshal(c.Plain, &v)
	if v.Progress == nil || *v.Progress < 0.41 || *v.Progress > 0.43 {
		t.Fatalf("published progress = %v, want about 0.42", v.Progress)
	}
}

// A download that goes final leaves the watch set. Its outcome must still be
// announced — once — and then it must stop costing anything.
func TestWatch_AFinishedDownloadIsAnnouncedOnceThenForgotten(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, 1, true)
	d.reconcileYtdlOnce(context.Background())
	drain()

	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())

	c, ok := changeFor(drain(), id)
	if !ok {
		t.Fatal("a download finished and nobody was told")
	}
	if s := stateIn(t, c); s != "completed" {
		t.Fatalf("published state = %q, want completed", s)
	}

	d.reconcileYtdlOnce(context.Background())
	if _, again := changeFor(drain(), id); again {
		t.Fatal("a finished download was announced twice; it cannot change again")
	}
	if _, held := d.ytdlSeen.seen[id]; held {
		t.Error("a finished download is still fingerprinted, so it costs a lookup every cycle forever")
	}
}

// FR-013. A dismissed download has no record left to read the owner from, so
// the owner has to have been remembered — otherwise a removal could only reach
// admins, and the person who dismissed it would keep the row.
func TestWatch_ADismissedDownloadIsRemovedForItsOwner(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	id := ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	owner := ownerOf(t, st, id)
	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, owner, false) // the OWNER, not an admin
	d.reconcileYtdlOnce(context.Background())
	drain()

	rec := do(t, h, "DELETE", "/v1/ytdl/"+id, "", admin)
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("dismiss = %d %s", rec.Code, rec.Body.String())
	}
	d.reconcileYtdlOnce(context.Background())

	c, ok := changeFor(drain(), id)
	if !ok {
		t.Fatal("a dismissed download was not announced to its owner; the row would linger")
	}
	if !c.Removed {
		t.Fatalf("published %+v, want a removal", c)
	}
}

// FR-004a. A client inserts a new top-level row and ignores an update about
// something it has never loaded, so the two have to be distinguishable — and an
// item has to say it belongs behind a group.
func TestWatch_NewAndNestedAreDistinguishable(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	drain := watchOn(t, d, 1, true)
	d.reconcileYtdlOnce(context.Background()) // seed on an empty instance
	drain()

	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	d.reconcileYtdlOnce(context.Background())

	c, ok := changeFor(drain(), sub.RequestID)
	if !ok {
		t.Fatal("a newly submitted download was not announced")
	}
	if !c.Created {
		t.Error("a download nobody has seen before is not marked created, so a watching admin would never see it appear")
	}
	if c.Item {
		t.Error("a top-level download is marked as an item")
	}
}

// FR-010 / SC-002a. The diff exists for watchers; with none, it must cost
// nothing at all.
func TestWatch_CostsNothingWhenNobodyIsWatching(t *testing.T) {
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, ytdlCfg())
	admin := adminAfterSetup(t, h)
	ytdlRunning(t, h, jobs, st, admin, "https://youtu.be/abc")

	d := InitCaches(Deps{Cfg: ytdlCfg(), Jobs: jobs, Store: st})
	d.reconcileYtdlOnce(context.Background())

	if len(d.ytdlSeen.seen) != 0 || d.ytdlSeen.spoken {
		t.Fatal("the diff ran and held state with nobody watching")
	}
}

// ownerOf reads a download's owner straight from the store.
func ownerOf(t *testing.T, st *store.Store, id string) int64 {
	t.Helper()
	rec, err := st.GetYtdlDownload(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	if rec.UserID == nil {
		t.Fatalf("%s has no owner", id)
	}
	return *rec.UserID
}

var _ = time.Second
