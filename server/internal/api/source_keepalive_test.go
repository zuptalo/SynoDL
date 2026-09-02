package api

import (
	"context"
	"testing"

	"synodl/server/internal/source"
	"synodl/server/internal/store"
)

func TestSourceKeepAliveProbe(t *testing.T) {
	resetFake()
	h, st := newStatefulRouter(t)
	admin := adminAfterSetup(t, h)
	configureFake(t, h, admin, "movie")
	d := Deps{Store: st}
	ctx := context.Background()

	p, _ := st.GetProvider()

	// Self-heal: a source stuck in needs_refresh after a transient blip goes back
	// to active on the next healthy probe (and the failure streak clears).
	_ = st.SetProviderState(p.ID, store.SourceNeedsRefresh, 0, 1)
	// The streak is per source now, so seed this source's.
	noteSourceFailure(p.ID)
	noteSourceFailure(p.ID)
	fakeVerifyErr = nil
	d.probeSource(ctx)
	if got, _ := st.GetProvider(); got.State != store.SourceActive {
		t.Fatalf("healthy probe should restore active, got %s", got.State)
	}
	if n := sourceFailureCount(p.ID); n != 0 {
		t.Fatalf("healthy probe should clear the streak, got %d", n)
	}

	// A genuine auth failure only flips to needs_refresh after the threshold — a
	// lone failed probe leaves the session intact.
	fakeVerifyErr = &source.ErrProviderVerify{Reason: "invalid_token"}
	for i := 0; i < sourceFailThreshold-1; i++ {
		d.probeSource(ctx)
	}
	if got, _ := st.GetProvider(); got.State != store.SourceActive {
		t.Fatalf("below-threshold probes must keep active, got %s", got.State)
	}
	d.probeSource(ctx) // the threshold-th consecutive failure
	if got, _ := st.GetProvider(); got.State != store.SourceNeedsRefresh {
		t.Fatalf("threshold probe should flip needs_refresh, got %s", got.State)
	}

	// A network/infra error (not an auth failure) must NOT count toward expiry.
	resetFake()
	_ = st.SetProviderState(p.ID, store.SourceActive, 1, 1)
	fakeVerifyErr = context.DeadlineExceeded
	for i := 0; i < sourceFailThreshold+2; i++ {
		d.probeSource(ctx)
	}
	if got, _ := st.GetProvider(); got.State != store.SourceActive {
		t.Fatalf("transient network errors must not flip the state, got %s", got.State)
	}

	// No configured provider → the probe is a harmless no-op.
	_ = st.DeleteProvider(p.ID)
	d.probeSource(ctx)
}
