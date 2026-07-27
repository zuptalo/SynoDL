package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("unit-test-passphrase")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(h, "unit-test-passphrase") {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword(h, "wrong") {
		t.Fatal("wrong password must not verify")
	}
}

func TestHashIsSaltedAndOpaque(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("same password must hash differently (random salt)")
	}
	for _, h := range []string{a, b} {
		if len(h) < 40 || h == "same" {
			t.Fatalf("hash looks wrong / leaks plaintext: %q", h)
		}
	}
}

func TestVerifyRejectsGarbageHash(t *testing.T) {
	for _, bad := range []string{"", "notahash", "pbkdf2-sha256$x$y", "md5$1$a$b"} {
		if VerifyPassword(bad, "anything") {
			t.Fatalf("garbage hash %q must not verify", bad)
		}
	}
}

func TestSessionTokens(t *testing.T) {
	tok1, hash1, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	tok2, hash2, _ := NewSessionToken()
	if tok1 == tok2 || hash1 == hash2 {
		t.Fatal("tokens/hashes must be unique")
	}
	if tok1 == hash1 {
		t.Fatal("the stored hash must differ from the raw token")
	}
	if HashToken(tok1) != hash1 {
		t.Fatal("HashToken must reproduce the stored hash")
	}
}
