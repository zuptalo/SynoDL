package api

import (
	"path/filepath"
	"testing"
	"time"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

func ownershipStore(t *testing.T) *store.Store {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// attribute durably records that userID owns the DSM task (what the watcher does
// once it first sees the task).
func attribute(t *testing.T, st *store.Store, taskID string, userID int64) {
	t.Helper()
	if err := st.UpsertWatched(taskID, "downloading", false); err != nil {
		t.Fatalf("UpsertWatched: %v", err)
	}
	if err := st.SetWatchedOwner(taskID, userID); err != nil {
		t.Fatalf("SetWatchedOwner: %v", err)
	}
}

func names(views []taskView) map[string]string {
	out := map[string]string{}
	for _, v := range views {
		out[v.ID] = v.AddedBy
	}
	return out
}

func TestDecorateTasksOwnershipAndAttribution(t *testing.T) {
	st := ownershipStore(t)
	adminID, _ := st.CreateUser("boss", "h", true)
	bobID, _ := st.CreateUser("bob", "h", false)
	admin, _ := st.GetUserByID(adminID)
	bob, _ := st.GetUserByID(bobID)
	d := Deps{Store: st}

	tasks := []syno.Task{
		{ID: "t1", Name: "bobs.iso"},
		{ID: "t2", Name: "bosss.iso"},
		{ID: "t3", Name: "orphan.iso"},
	}
	attribute(t, st, "t1", bobID)
	attribute(t, st, "t2", adminID)
	// t3 stays unattributed (e.g. added on the NAS directly).

	// Non-admin bob sees only his own task, with no "added by" leaked.
	bobView := names(d.decorateTasks(bob, tasks))
	if len(bobView) != 1 {
		t.Fatalf("bob should see exactly his own task, got %v", bobView)
	}
	if _, ok := bobView["t1"]; !ok {
		t.Fatalf("bob must see t1, got %v", bobView)
	}
	if bobView["t1"] != "" {
		t.Fatalf("non-admin must not get an addedBy label, got %q", bobView["t1"])
	}

	// Admin (default scope "any") sees everything, labelled by owner.
	adminView := names(d.decorateTasks(admin, tasks))
	if len(adminView) != 3 {
		t.Fatalf("admin should see all three tasks, got %v", adminView)
	}
	if adminView["t1"] != "bob" || adminView["t2"] != "boss" {
		t.Fatalf("addedBy labels wrong: %v", adminView)
	}
	if adminView["t3"] != "" {
		t.Fatalf("unattributed task must have no addedBy, got %q", adminView["t3"])
	}

	// Admin who opts down to "own" sees only their own task.
	_ = st.SaveNotificationPrefs(adminID, store.NotificationPrefs{NotifyCompleted: true, Scope: "own"}, 0)
	ownView := names(d.decorateTasks(admin, tasks))
	if len(ownView) != 1 || ownView["t2"] != "boss" {
		t.Fatalf("admin in own-scope should see only t2, got %v", ownView)
	}
}

// A just-created task shows for its creator via a pending claim, before the
// watcher has durably attributed it.
func TestDecorateTasksPendingClaimBridge(t *testing.T) {
	st := ownershipStore(t)
	bobID, _ := st.CreateUser("bob", "h", false)
	bob, _ := st.GetUserByID(bobID)
	d := Deps{Store: st}

	// Bob just added "fresh.iso" — a claim exists but the watcher hasn't attributed
	// the DSM task yet.
	if err := st.AddTaskClaim(bobID, "fresh.iso", time.Now().Unix()); err != nil {
		t.Fatalf("AddTaskClaim: %v", err)
	}
	tasks := []syno.Task{{ID: "t9", Name: "fresh.iso"}}
	view := names(d.decorateTasks(bob, tasks))
	if _, ok := view["t9"]; !ok {
		t.Fatalf("bob should see his freshly-created (still-unattributed) task, got %v", view)
	}
}
