package api

import (
	"context"
	"time"

	"synodl/server/internal/source"
	"synodl/server/internal/store"
)

// RunSourceKeepAlive periodically probes the configured download source with a
// single lightweight authenticated call. It:
//   - keeps the session warm so it doesn't idle-expire;
//   - self-heals a source that got stuck in needs_refresh after a transient blip
//     (a successful probe restores it to active);
//   - only after several CONSECUTIVE genuine auth failures flips it to
//     needs_refresh, so an admin is prompted to re-paste BEFORE a user hits it
//     cold — not on a lone transient failure.
//
// One call per interval keeps it gentle enough not to trip the provider's rate
// limiting. Runs until ctx is cancelled.
func (d Deps) RunSourceKeepAlive(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.probeSource(ctx)
		}
	}
}

// probeSource runs one keep-alive probe. Shares the same consecutive-failure
// hysteresis (sourceFailStreak) as the request paths, so a probe success clears a
// streak the user's requests started, and vice versa.
func (d Deps) probeSource(ctx context.Context) {
	p, drv, cfg, sess, ok := d.activeSource()
	if !ok {
		return // no enabled provider — nothing to keep warm
	}
	err := drv.VerifySession(ctx, sourceHTTP, cfg, sess)
	if err == nil {
		d.sourceCallOK(p) // healthy: reset the streak, restore active, stamp last_verified
		return
	}
	// Only a genuine auth/verify failure counts toward expiry; a network/infra
	// error is transient and ignored (the next probe retries).
	var ve *source.ErrProviderVerify
	if !asProviderVerify(err, &ve) {
		return
	}
	if sourceFailStreak.Add(1) >= sourceFailThreshold {
		_ = d.Store.SetProviderState(p.ID, store.SourceNeedsRefresh, 0, time.Now().Unix())
	}
}
