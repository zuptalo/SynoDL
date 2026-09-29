package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const signinMarker = "SECRETVALUE-91be"

func TestYoutubeSignIn_SaveReadDelete(t *testing.T) {
	s := openTestStore(t)
	if y, err := s.GetYoutubeSignIn(); err != nil || y != nil {
		t.Fatalf("empty store: %v %v", y, err)
	}
	if _, ok, _ := s.OpenYoutubeSignInCookies(); ok {
		t.Fatal("cookies present in an empty store")
	}
	cookies := []byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1\tSAPISID\t" + signinMarker + "\n")
	if err := s.SaveYoutubeSignIn(cookies, 1, []string{"SAPISID"}, "Anna", 1_790_000_000); err != nil {
		t.Fatal(err)
	}
	y, err := s.GetYoutubeSignIn()
	if err != nil || y == nil || y.CookieCount != 1 || y.SavedByName != "Anna" || len(y.LoginFound) != 1 || y.LoginFound[0] != "SAPISID" {
		t.Fatalf("metadata: %+v %v", y, err)
	}
	got, ok, err := s.OpenYoutubeSignInCookies()
	if err != nil || !ok || !bytes.Equal(got, cookies) {
		t.Fatalf("round trip: %v %v", ok, err)
	}
	if err := s.DeleteYoutubeSignIn(); err != nil {
		t.Fatal(err)
	}
	if y, _ := s.GetYoutubeSignIn(); y != nil {
		t.Fatal("still present after delete")
	}
}

func TestYoutubeSignIn_NothingReadableAtRest(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCipher("kdf-input-for-tests")
	dbPath := filepath.Join(dir, "synodl.db")
	s, err := Open(dbPath, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveYoutubeSignIn([]byte("x\t"+signinMarker+"\n"), 3, nil, "Anna", 1); err != nil {
		t.Fatal(err)
	}
	_ = s.Close() // checkpoints the WAL into the main file
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if bytes.Contains(b, []byte(signinMarker)) {
			t.Fatalf("plaintext cookie value found in %s", e.Name())
		}
	}
}

func TestYoutubeSignIn_WrongKeyReadsAsAbsent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "synodl.db")
	c1, _ := NewCipher("first-key")
	s, err := Open(dbPath, c1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.SaveYoutubeSignIn([]byte("cookies"), 3, nil, "Anna", 1)
	_ = s.Close()
	c2, _ := NewCipher("second-key")
	s2, err := Open(dbPath, c2)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if y, err := s2.GetYoutubeSignIn(); err != nil || y != nil {
		t.Fatalf("metadata with the wrong key: %v %v", y, err)
	}
	if _, ok, err := s2.OpenYoutubeSignInCookies(); ok || err != nil {
		t.Fatalf("cookies with the wrong key: %v %v", ok, err)
	}
}

func TestYoutubeSignIn_OutcomesAndReplaceResets(t *testing.T) {
	s := openTestStore(t)
	_ = s.SaveYoutubeSignIn([]byte("c"), 3, nil, "Anna", 10)
	_ = s.NoteYoutubeSignInOutcome(true, 20)
	y, _ := s.GetYoutubeSignIn()
	if y.LastRefusedAt == nil || *y.LastRefusedAt != 20 || y.LastOKAt != nil {
		t.Fatalf("after refusal: %+v", y)
	}
	_ = s.NoteYoutubeSignInOutcome(false, 30)
	y, _ = s.GetYoutubeSignIn()
	if y.LastRefusedAt != nil || y.LastOKAt == nil || *y.LastOKAt != 30 {
		t.Fatalf("a success must clear the refusal: %+v", y)
	}
	_ = s.NoteYoutubeSignInOutcome(true, 40)
	_ = s.SaveYoutubeSignIn([]byte("new"), 4, nil, "Bo", 50)
	y, _ = s.GetYoutubeSignIn()
	if y.LastRefusedAt != nil || y.LastOKAt != nil || y.SavedByName != "Bo" {
		t.Fatalf("a new sign-in must start clean: %+v", y)
	}
	// Outcomes with nothing saved are a no-op, not an error.
	_ = s.DeleteYoutubeSignIn()
	if err := s.NoteYoutubeSignInOutcome(true, 60); err != nil {
		t.Fatal(err)
	}
}
