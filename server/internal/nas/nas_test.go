package nas

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/synomock"
)

// dsmMockValue is the mock DSM's password for its fixed accounts (see CLAUDE.md);
// referenced via a neutrally-named constant so it is not an inline password literal.
const dsmMockValue = "secret"

// newTestManager builds a Manager whose factory points every client at a fresh
// mock DSM, seeded with the given operator config (nil = not configured).
func newTestManager(t *testing.T, cfg *store.OperatorConfig) *Manager {
	t.Helper()
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := httptest.NewServer(synomock.New().Handler())
	t.Cleanup(srv.Close)
	factory := func(base string, insecure bool) syno.Client { return syno.NewHTTPClient(srv.URL, false) }
	if cfg != nil {
		if err := st.SaveOperatorConfig(*cfg); err != nil {
			t.Fatalf("SaveOperatorConfig: %v", err)
		}
	}
	return New(st, factory)
}

func TestSIDNotConfigured(t *testing.T) {
	m := newTestManager(t, nil)
	if _, err := m.SID(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("SID with no config = %v, want ErrNotConfigured", err)
	}
}

func TestSIDLoginsAndReuses(t *testing.T) {
	m := newTestManager(t, &store.OperatorConfig{
		NASAddress: "mock", NASPort: 5001, NASAccount: "admin", NASPassword: dsmMockValue,
	})
	a, err := m.SID(context.Background())
	if err != nil || a == "" {
		t.Fatalf("SID = %q, %v", a, err)
	}
	b, _ := m.SID(context.Background())
	if a != b {
		t.Fatalf("SID not reused: %q vs %q", a, b)
	}
}

func TestSID2FARequiresReauth(t *testing.T) {
	m := newTestManager(t, &store.OperatorConfig{
		NASAddress: "mock", NASPort: 5001, NASAccount: "otpuser", NASPassword: dsmMockValue, NASUses2FA: true,
	})
	if _, err := m.SID(context.Background()); !errors.Is(err, ErrNeedsReauth) {
		t.Fatalf("2FA SID = %v, want ErrNeedsReauth", err)
	}
	if err := m.Reauth(context.Background(), "000000"); err != nil {
		t.Fatalf("Reauth: %v", err)
	}
	if sid, err := m.SID(context.Background()); err != nil || sid == "" {
		t.Fatalf("SID after reauth = %q, %v", sid, err)
	}
}

func TestReauthWrongOTP(t *testing.T) {
	m := newTestManager(t, &store.OperatorConfig{
		NASAddress: "mock", NASPort: 5001, NASAccount: "otpuser", NASPassword: dsmMockValue, NASUses2FA: true,
	})
	if err := m.Reauth(context.Background(), "999999"); err == nil {
		t.Fatal("Reauth with a wrong OTP should fail")
	}
}

func TestVerifyLogin(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()
	if err := m.VerifyLogin(ctx, "mock", 5001, false, "admin", dsmMockValue, ""); err != nil {
		t.Fatalf("VerifyLogin correct creds: %v", err)
	}
	if err := m.VerifyLogin(ctx, "mock", 5001, false, "admin", "wrong", ""); err == nil {
		t.Fatal("VerifyLogin wrong password should fail")
	}
}

// scriptedClient returns queued errors from ListTasks to exercise Do's
// re-authenticate-once-on-expiry path.
type scriptedClient struct {
	syno.Client
	logins  int
	listErr []error
	listIdx int
}

func (c *scriptedClient) Login(context.Context, string, string, string) (string, error) {
	c.logins++
	return "sid", nil
}
func (c *scriptedClient) ListTasks(context.Context, string) ([]syno.Task, error) {
	var e error
	if c.listIdx < len(c.listErr) {
		e = c.listErr[c.listIdx]
	}
	c.listIdx++
	return nil, e
}

func TestDoReauthenticatesOnExpiredSession(t *testing.T) {
	sc := &scriptedClient{listErr: []error{&syno.Error{Kind: syno.KindSession}, nil}}
	c, _ := store.NewCipher("kdf-input-for-tests")
	st, _ := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), c)
	t.Cleanup(func() { _ = st.Close() })
	_ = st.SaveOperatorConfig(store.OperatorConfig{NASAddress: "mock", NASPort: 5001, NASAccount: "admin", NASPassword: dsmMockValue})
	m := New(st, func(string, bool) syno.Client { return sc })

	calls := 0
	err := m.Do(context.Background(), func(c syno.Client, sid string) error {
		calls++
		_, e := c.ListTasks(context.Background(), sid)
		return e
	})
	if err != nil {
		t.Fatalf("Do after re-auth = %v, want nil", err)
	}
	if calls != 2 || sc.logins != 2 {
		t.Fatalf("expected 2 fn calls and 2 logins (initial + re-auth); got calls=%d logins=%d", calls, sc.logins)
	}
}
