package store

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Opaque list cursors (spec 0013, FR-006a).
//
// History is unbounded, so the list is paged by a keyset rather than by OFFSET —
// OFFSET re-walks every row it skips, which turns "show me page 40 of a channel
// I expanded" into a scan of everything before it.
//
// The cursor is base64 only so it reads as opaque and callers do not start
// parsing it. It carries no secret and grants nothing: it is a position in the
// caller's OWN result set, and the ownership filter is applied independently on
// every request — a cursor lifted from someone else's response selects rows the
// filter then excludes.

func encodeYtdlCursor(createdAt int64, requestID string) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(strconv.FormatInt(createdAt, 10) + ":" + requestID))
}

func decodeYtdlCursor(c string) (int64, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, "", false
	}
	at, id, ok := strings.Cut(string(raw), ":")
	if !ok {
		return 0, "", false
	}
	n, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return n, id, true
}

func encodeYtdlSeqCursor(seq int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(seq, 10)))
}

func decodeYtdlSeqCursor(c string) (int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
