package store

import (
	"errors"
	"testing"
	"time"
)

func dl(id string, uid *int64) YtdlDownload {
	return YtdlDownload{
		RequestID: id, Kind: YtdlKindSingle, UserID: uid,
		SourceURL: "https://youtu.be/" + id, VideoID: id,
		Mode: "music", Scope: "single", State: "queued",
		CreatedAt: 1764700000,
	}
}

func TestCreateAndGetYtdlDownload(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)

	in := dl("aaa", &anna)
	in.Title = "Bohemian Rhapsody"
	in.Uploader = "Queen"
	if err := s.CreateYtdlDownload(in); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetYtdlDownload("aaa")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Bohemian Rhapsody" || got.Uploader != "Queen" || got.Mode != "music" {
		t.Fatalf("round-trip lost fields: %+v", got)
	}
	// FR-033: not finished means absent, never zero.
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil until the download is final", *got.FinishedAt)
	}
	if got.Attempts != 1 {
		t.Errorf("Attempts = %d, want the first attempt to count as one", got.Attempts)
	}

	if _, err := s.GetYtdlDownload("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing download err = %v, want ErrNotFound", err)
	}
}

func TestListYtdlDownloads_ScopesAndPages(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)
	bo, _ := s.CreateUser("bo", "h", false)

	for i, id := range []string{"a1", "a2", "a3"} {
		d := dl(id, &anna)
		d.CreatedAt = int64(1764700000 + i)
		if err := s.CreateYtdlDownload(d); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if err := s.CreateYtdlDownload(dl("b1", &bo)); err != nil {
		t.Fatalf("create b1: %v", err)
	}

	mine, _, err := s.ListYtdlDownloads(anna, false, "", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 3 {
		t.Fatalf("anna sees %d, want only her three", len(mine))
	}

	all, _, err := s.ListYtdlDownloads(anna, true, "", 50)
	if err != nil {
		t.Fatalf("list as admin: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("admin sees %d, want everyone's four", len(all))
	}

	// Paging walks the whole set without repeating or dropping a row — the
	// property that matters when a channel has expanded to thousands (FR-006a).
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 10; page++ {
		got, next, err := s.ListYtdlDownloads(anna, false, cursor, 2)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, d := range got {
			if seen[d.RequestID] {
				t.Fatalf("%s returned on two pages", d.RequestID)
			}
			seen[d.RequestID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 3 {
		t.Fatalf("paging saw %d rows, want all 3", len(seen))
	}
}

func TestListYtdlDownloads_ExcludesGroupItems(t *testing.T) {
	// FR-019b. A group is ONE row in the list; its items live behind it, so an
	// uncapped channel cannot crowd out a user's other downloads.
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)

	group := dl("grp", &anna)
	group.Kind = YtdlKindGroup
	group.Scope = "channel"
	if err := s.CreateYtdlDownload(group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, id := range []string{"i1", "i2", "i3"} {
		item := dl(id, &anna)
		item.Kind = YtdlKindItem
		item.ParentID = "grp"
		item.Origin = YtdlOriginExpanded
		if err := s.CreateYtdlDownload(item); err != nil {
			t.Fatalf("create item %s: %v", id, err)
		}
	}

	top, _, err := s.ListYtdlDownloads(anna, false, "", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(top) != 1 || top[0].RequestID != "grp" {
		t.Fatalf("top level = %+v, want just the group row", top)
	}

	items, _, err := s.ListYtdlItems("grp", "", 50)
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("group has %d items, want 3", len(items))
	}
}

func TestYtdlCounts_AggregatesAGroup(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)
	group := dl("grp", &anna)
	group.Kind = YtdlKindGroup
	_ = s.CreateYtdlDownload(group)

	for id, state := range map[string]string{"i1": "completed", "i2": "completed", "i3": "failed", "i4": "queued"} {
		item := dl(id, &anna)
		item.Kind, item.ParentID, item.State = YtdlKindItem, "grp", state
		_ = s.CreateYtdlDownload(item)
	}

	c, err := s.YtdlCounts("grp")
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if c.Total != 4 || c.Completed != 2 || c.Failed != 1 || c.Remaining != 1 {
		t.Fatalf("counts = %+v, want 4 total / 2 saved / 1 failed / 1 remaining", c)
	}
}

func TestYtdlAlreadyHeld_OnlyCountsCompleted(t *testing.T) {
	// FR-020. Re-running a channel must skip what it already has — and only a
	// COMPLETED record means "already has".
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)

	done := dl("song1", &anna)
	done.State = "completed"
	_ = s.CreateYtdlDownload(done)

	failed := dl("song2", &anna)
	failed.State = "failed"
	_ = s.CreateYtdlDownload(failed)

	for _, tc := range []struct {
		video, mode string
		want        bool
	}{
		{"song1", "music", true},
		{"song1", "music-video", false}, // a different library entirely
		{"song2", "music", false},       // failed is not held
		{"never", "music", false},
		{"", "music", false},
	} {
		got, err := s.YtdlAlreadyHeld(tc.video, tc.mode)
		if err != nil {
			t.Fatalf("held(%s,%s): %v", tc.video, tc.mode, err)
		}
		if got != tc.want {
			t.Errorf("held(%s,%s) = %v, want %v", tc.video, tc.mode, got, tc.want)
		}
	}
}

func TestDeleteYtdlDownload_CascadesAndRespectsOwnership(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)
	bo, _ := s.CreateUser("bo", "h", false)

	group := dl("grp", &anna)
	group.Kind = YtdlKindGroup
	_ = s.CreateYtdlDownload(group)
	for _, id := range []string{"i1", "i2"} {
		item := dl(id, &anna)
		item.Kind, item.ParentID = YtdlKindItem, "grp"
		_ = s.CreateYtdlDownload(item)
	}

	// Not yours behaves exactly like not there (FR-008).
	notYours, err := s.DeleteYtdlDownload("grp", bo, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	notThere, err := s.DeleteYtdlDownload("never", bo, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if notYours || notThere {
		t.Fatalf("removed = (%v, %v), want both false and indistinguishable", notYours, notThere)
	}

	// The owner dismisses the group, and its items go with it (FR-005a).
	ok, err := s.DeleteYtdlDownload("grp", anna, false)
	if err != nil || !ok {
		t.Fatalf("owner delete = (%v, %v), want it removed", ok, err)
	}
	items, _, _ := s.ListYtdlItems("grp", "", 50)
	if len(items) != 0 {
		t.Fatalf("%d items survived their group, want the cascade to take them", len(items))
	}
}

func TestYtdlDownload_SurvivesTheAccountThatMadeIt(t *testing.T) {
	// FR-006d. Deleting an account does not erase the record that a download
	// happened — matching how NAS tasks and stored failures already behave. The
	// row survives unattributed, and is then admin-visible only.
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)
	_ = s.CreateYtdlDownload(dl("aaa", &anna))

	if err := s.DeleteUser(anna); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	got, err := s.GetYtdlDownload("aaa")
	if err != nil {
		t.Fatalf("get after account deletion: %v", err)
	}
	if got.UserID != nil {
		t.Fatalf("UserID = %d, want it detached rather than dangling", *got.UserID)
	}

	// Nobody owns it now, so only an admin sees it.
	other, _ := s.CreateUser("bo", "h", false)
	mine, _, _ := s.ListYtdlDownloads(other, false, "", 50)
	if len(mine) != 0 {
		t.Fatalf("a non-admin sees %d unattributed rows, want none", len(mine))
	}
	all, _, _ := s.ListYtdlDownloads(other, true, "", 50)
	if len(all) != 1 {
		t.Fatalf("an admin sees %d rows, want the unattributed one", len(all))
	}
}

func TestSetYtdlStateAndCompanion(t *testing.T) {
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)
	_ = s.CreateYtdlDownload(dl("aaa", &anna))

	at := int64(1764700500)
	if err := s.SetYtdlState("aaa", "failed", "the download did not complete", &at); err != nil {
		t.Fatalf("set state: %v", err)
	}
	got, _ := s.GetYtdlDownload("aaa")
	if got.State != "failed" || got.Reason == "" || got.FinishedAt == nil || *got.FinishedAt != at {
		t.Fatalf("after failure: %+v", got)
	}

	// FR-013g: what the worker produced, captured while its output still exists.
	if err := s.SetYtdlCompanion("aaa", true, "en"); err != nil {
		t.Fatalf("set companion: %v", err)
	}
	got, _ = s.GetYtdlDownload("aaa")
	if got.HasLyrics == nil || !*got.HasLyrics || got.LyricsLang != "en" {
		t.Fatalf("companion facts not recorded: %+v", got)
	}
}

// SC-006a. History is unbounded by design (FR-006), so the list has to stay
// responsive at a size a single expanded channel can reach on its own. The
// property under test is that a page costs the same whether it is the first
// page of ten rows or the fortieth of ten thousand — which is the difference
// between a keyset cursor and OFFSET, and the reason the cursor exists.
func TestListYtdlDownloads_StaysFastAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds thousands of rows")
	}
	s := openTestStore(t)
	anna, _ := s.CreateUser("anna", "h", false)

	const rows = 5000
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := tx.Exec(
			`INSERT INTO ytdl_downloads (request_id, kind, user_id, source_url, video_id, mode, scope, state, queue_seq, created_at)
			 VALUES (?, 'single', ?, ?, ?, 'music', 'single', 'completed', ?, ?)`,
			"r"+itoa(i), anna, "https://youtu.be/v"+itoa(i), "v"+itoa(i), i, 1700000000+i,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	first := time.Now()
	page, cursor, err := s.ListYtdlDownloads(anna, false, "", 50)
	firstPage := time.Since(first)
	if err != nil || len(page) != 50 {
		t.Fatalf("first page: %d rows, %v", len(page), err)
	}

	// Walk deep into the history — the case OFFSET degrades on, because it
	// re-walks everything it skips.
	for i := 0; i < 60 && cursor != ""; i++ {
		page, cursor, err = s.ListYtdlDownloads(anna, false, cursor, 50)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
	}
	deep := time.Now()
	_, _, err = s.ListYtdlDownloads(anna, false, cursor, 50)
	deepPage := time.Since(deep)
	if err != nil {
		t.Fatalf("deep page: %v", err)
	}

	// Generous by an order of magnitude: this is a shape check, not a
	// benchmark, and it should not go red because the machine was busy.
	if deepPage > 20*firstPage+50*time.Millisecond {
		t.Fatalf("a deep page took %v against a first page of %v — the list is not paging by keyset", deepPage, firstPage)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The watch set is what a live update can be about, so what it leaves out
// matters as much as what it holds: a finished download cannot change again.
func TestListYtdlUnfinished(t *testing.T) {
	st := openTestStore(t)
	anna, _ := st.CreateUser("anna", "h", false)

	mk := func(id string, kind YtdlKind, state string) {
		t.Helper()
		d := dl(id, &anna)
		d.Kind = kind
		d.State = state
		if err := st.CreateYtdlDownload(d); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("a", YtdlKindSingle, "queued")
	mk("b", YtdlKindGroup, "resolving")
	mk("c", YtdlKindSingle, "scheduled")
	mk("d", YtdlKindSingle, "downloading")
	mk("e", YtdlKindGroup, "downloading")
	mk("f", YtdlKindSingle, "completed")
	mk("g", YtdlKindSingle, "failed")

	got, err := st.ListYtdlUnfinished()
	if err != nil {
		t.Fatalf("ListYtdlUnfinished: %v", err)
	}
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.RequestID] = true
	}
	for _, want := range []string{"a", "b", "c", "d", "e"} {
		if !ids[want] {
			t.Errorf("watch set is missing %q — its state can still change", want)
		}
	}
	for _, unwanted := range []string{"f", "g"} {
		if ids[unwanted] {
			t.Errorf("watch set holds %q, which is final and cannot change again", unwanted)
		}
	}
}
