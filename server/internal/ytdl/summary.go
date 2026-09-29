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
	case ReasonSignInRefused:
		return "turned away — the YouTube sign-in needs replacing"
	}
	return "did not complete"
}

// Permanent reports whether a failure reason is one a retry cannot change.
//
// YouTube removing a video, or gating it behind age, payment or region, is a
// fact about the video; only a refusal (a bot check, a 403/429) is a fact about
// the moment. "The download did not complete" says nothing either way, so it is
// treated as retryable — dismissing on a guess would throw away something that
// might well save next time.
func Permanent(reason string) bool {
	switch reason {
	case ReasonUnavailable, ReasonAgeRestricted, ReasonPaid, ReasonRegion:
		return true
	}
	return false
}

// AllPermanent reports whether every counted failure is permanent — the test
// for a playlist that can never reach Finished, however often it is retried
// (spec 1050). No failures at all is not "all permanent": there is nothing to
// give up on.
func AllPermanent(counts []ReasonCount) bool {
	total := 0
	for _, c := range counts {
		if c.N <= 0 {
			continue
		}
		if !Permanent(c.Reason) {
			return false
		}
		total += c.N
	}
	return total > 0
}
