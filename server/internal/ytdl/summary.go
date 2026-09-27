package ytdl

import (
	"fmt"
	"sort"
	"strings"
)

// Why a playlist failed, in one line (spec 1048).
//
// A playlist used to end with "some items could not be downloaded", which said
// nothing about whether trying again was worth anything. Its tracks each carry
// a reason (spec 2035); this folds them into a line a person reads at a glance:
//
//	4 could not be downloaded: 3 no longer available, 1 adults only
//
// so a playlist whose failures are all permanent is visibly not worth a retry,
// and one that was merely turned away visibly is.

// ReasonCount is how many tracks failed for one reason.
type ReasonCount struct {
	Reason string
	N      int
}

// SummarizeFailures builds the line. Reasons are named by their short form and
// listed most common first; an unrecognised reason reads as "did not complete".
func SummarizeFailures(counts []ReasonCount) string {
	total := 0
	byPhrase := map[string]int{}
	for _, c := range counts {
		if c.N <= 0 {
			continue
		}
		total += c.N
		byPhrase[shortReason(c.Reason)] += c.N
	}
	if total == 0 {
		return ""
	}
	type part struct {
		phrase string
		n      int
	}
	parts := make([]part, 0, len(byPhrase))
	for p, n := range byPhrase {
		parts = append(parts, part{p, n})
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].n != parts[j].n {
			return parts[i].n > parts[j].n
		}
		return parts[i].phrase < parts[j].phrase
	})
	head := fmt.Sprintf("%d could not be downloaded: ", total)
	if len(parts) == 1 {
		return head + parts[0].phrase
	}
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		items = append(items, fmt.Sprintf("%d %s", p.n, p.phrase))
	}
	return head + strings.Join(items, ", ")
}

// shortReason is the noun phrase for a track's reason — what fits after a
// count in a list.
func shortReason(reason string) string {
	switch reason {
	case ReasonUnavailable:
		return "no longer available"
	case ReasonAgeRestricted:
		return "adults only"
	case ReasonPaid:
		return "paying members only"
	case ReasonRegion:
		return "not offered in this region"
	case ReasonRefused:
		return "turned away by YouTube"
	case ReasonRefusedRetrying:
		return "waiting to retry"
	}
	return "did not complete"
}
