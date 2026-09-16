package people

import (
	"context"
	"net/url"
	"testing"
)

// The helpers below let a test stand in for IMDb without the production build
// ever gaining a way to do so: they swap the host rule for the duration of one
// test, in test code, which a release binary does not contain.

func allowHost(t *testing.T, base string) {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("bad base %q: %v", base, err)
	}
	prev := testHostOverride
	testHostOverride = u.Hostname()
	t.Cleanup(func() { testHostOverride = prev })
}

func readHeadAllowingHost(t *testing.T, raw string) ([]byte, error) {
	t.Helper()
	allowHost(t, raw)
	return readHead(context.Background(), raw)
}

func lookupAllowingHost(t *testing.T, base, imdbID string) (string, error) {
	t.Helper()
	allowHost(t, base)
	prev := testBaseOverride
	testBaseOverride = base
	t.Cleanup(func() { testBaseOverride = prev })
	return LookupPhoto(context.Background(), imdbID)
}
