package api

import (
	"net/url"
	"strings"
)

// titleHint derives the name DSM will most likely give a task from its source
// URI, so a freshly-created task can be attributed to its creator when it later
// appears in the task list (DSM's create call returns no id, so we can't record
// ownership directly). Mirrors DSM's usual behavior for direct downloads
// (filename) and magnets (the dn= display name). Best-effort — attribution only
// affects which users get an opt-in notification, never access.
func titleHint(uri string) string {
	uri = strings.TrimSpace(uri)
	if strings.HasPrefix(uri, "magnet:") {
		for _, kv := range strings.Split(strings.TrimPrefix(uri, "magnet:?"), "&") {
			if v, found := strings.CutPrefix(kv, "dn="); found {
				if dec, err := url.QueryUnescape(v); err == nil {
					return dec
				}
				return v
			}
		}
		return "magnet download"
	}
	trimmed := strings.TrimRight(uri, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 && i < len(trimmed)-1 {
		return trimmed[i+1:]
	}
	return uri
}
