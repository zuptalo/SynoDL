package api

import (
	"errors"
	"net/url"
	"strings"
)

// errBadEpisode is returned when an episode selection references an episode that
// doesn't exist in the resolved season.
var errBadEpisode = errors.New("api: episode out of range")

// selectEpisodes narrows a series' per-episode links to the picked episodes
// (1-based, in `links` order). An empty pick — or a single-file title (a movie) —
// sends everything. Out-of-range or zero indices are rejected; duplicates are
// collapsed and the result stays in episode order.
func selectEpisodes(links []string, episodes []int) ([]string, error) {
	if len(episodes) == 0 || len(links) <= 1 {
		return links, nil
	}
	want := make(map[int]bool, len(episodes))
	for _, e := range episodes {
		if e < 1 || e > len(links) {
			return nil, errBadEpisode
		}
		want[e] = true
	}
	out := make([]string, 0, len(want))
	for i, link := range links {
		if want[i+1] {
			out = append(out, link)
		}
	}
	return out, nil
}

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
