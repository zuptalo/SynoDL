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

// probeSource runs one keep-alive probe per configured source. Each shares the
// same per-source consecutive-failure hysteresis as the request paths, so a
// probe success clears a streak the user's requests started, and vice versa.
//
// Every enabled source is probed, not just the first: a second source's session
// expires exactly as readily as the first one's, and an unprobed source would
// only reveal its expiry when a user hit it.
func (d Deps) probeSource(ctx context.Context) {
	refs, _ := d.sourceRefs()
	for _, ref := range refs {
		err := ref.Driver.VerifySession(ctx, sourceHTTP, ref.Cfg, ref.Sess)
		if err == nil {
			// Healthy: reset the streak, restore active, stamp last_verified.
			if p, e := d.Store.GetProviderByID(ref.ID); e == nil && p != nil {
				d.sourceCallOK(p)
			}
			continue
		}
		// Only a genuine auth/verify failure counts toward expiry; a network/infra
		// error is transient and ignored (the next probe retries).
		var ve *source.ErrProviderVerify
		if !asProviderVerify(err, &ve) {
			continue
		}
		// An entitlement problem is definite: record it immediately, since no
		// amount of waiting or re-pasting changes it.
		if ve.Reason == source.ReasonUnsubscribed {
			_ = d.Store.SetProviderStateErr(ref.ID, store.SourceUnsubscribed, ve.Reason, 0, time.Now().Unix())
			continue
		}
		if noteSourceFailure(ref.ID) >= sourceFailThreshold {
			_ = d.Store.SetProviderStateErr(ref.ID, store.SourceNeedsRefresh, ve.Reason, 0, time.Now().Unix())
		}
	}
}
