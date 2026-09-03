package providers

import (
	"sync"
	"time"

	"synodl/server/internal/source"
)

// Remembering which of a source's addresses is currently answering (spec 1020).
//
// Without this, every request during an outage pays for a failed attempt to the
// dead address before trying the mirror — turning a domain outage into a
// permanent latency penalty on every page of Discover.
//
// The memory is deliberately short-lived. When it lapses the main domain is
// tried again, so a source returns to its proper address on its own once the
// outage ends and the operator never has to do anything (FR-007).
const basePrefTTL = 5 * time.Minute

var (
	basePrefMu sync.Mutex
	basePrefs  = map[string]basePref{}
)

type basePref struct {
	base  string
	until time.Time
}

// basePrefKey scopes the memory to one configured source. Keying on the
// alternate address means two sources of the same kind, with different mirrors,
// never influence each other.
func basePrefKey(cfg source.Config) string { return cfg.AltBase }

// rememberWorkingBase records that base answered. Recording the primary CLEARS
// the preference rather than pinning it: the primary is the default, and
// pinning it would only delay noticing the next outage.
func rememberWorkingBase(cfg source.Config, base string) {
	key := basePrefKey(cfg)
	if key == "" {
		return
	}
	basePrefMu.Lock()
	defer basePrefMu.Unlock()
	if base != cfg.AltBase {
		delete(basePrefs, key)
		return
	}
	basePrefs[key] = basePref{base: base, until: time.Now().Add(basePrefTTL)}
}

// preferredBase returns the address to try first, or "" for the default order.
func preferredBase(cfg source.Config) string {
	key := basePrefKey(cfg)
	if key == "" {
		return ""
	}
	basePrefMu.Lock()
	defer basePrefMu.Unlock()
	p, ok := basePrefs[key]
	if !ok || time.Now().After(p.until) {
		return ""
	}
	return p.base
}

// ResetBasePrefs clears the memory. For tests, and for the moment an operator
// changes a source's addresses — a stale preference would otherwise outlive the
// configuration that produced it.
func ResetBasePrefs() {
	basePrefMu.Lock()
	defer basePrefMu.Unlock()
	basePrefs = map[string]basePref{}
}
