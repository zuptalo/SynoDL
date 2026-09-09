package ytdl

import "sort"

// Deciding what to start next (spec 0013, FR-021 – FR-022c).
//
// Spec 0012 had no queue at all, and said why: concurrency was "bounded by the
// cluster", with a job beyond available resources sitting Pending. That is a
// real bound, but it is invisible — a user cannot see it, cannot reorder it, and
// cannot tell it apart from a download that is simply slow. And once a channel
// can expand without a ceiling, "let the cluster sort it out" means handing it
// several hundred jobs at once.
//
// So admission is SynoDL's decision now, and this file is that decision as a
// pure function: no clock, no I/O, no database. Everything about who goes next
// is decided from a slice, which is why it can be exhaustively table-tested
// without a cluster anywhere in sight.

// Candidate is one queued download, reduced to what admission actually needs.
type Candidate struct {
	RequestID string
	// UserID identifies whose queue this belongs to. Zero means unattributed —
	// the account was deleted — and such a download is never admitted: nobody is
	// waiting for it.
	UserID int64
	// Direct is true for a link a user pasted, false for one produced by
	// expanding a playlist or channel.
	Direct bool
	// Seq is submission order, and the tiebreak of last resort.
	Seq int64
}

// Admit picks up to `slots` downloads to start, in the order they should start.
//
// Two rules, in this order:
//
//  1. Share between USERS (FR-022b). Slots rotate, so one person expanding a
//     500-item channel cannot make everybody else wait hours. Without this,
//     first-in-first-out would be correct and useless: the queue would be
//     technically fair and practically a denial of service by whoever pasted a
//     channel first.
//
//  2. Within one user, a PASTED link goes before their own expanded items
//     (FR-022c). Someone who queues a channel and then pastes a song wants the
//     song; the channel is background work they set off and walked away from.
//
// Rotation never idles a slot: if only one user has anything queued, they get
// every slot. The limit is a ceiling, not a per-user reservation.
func Admit(queued []Candidate, slots int) []Candidate {
	if slots <= 0 || len(queued) == 0 {
		return nil
	}

	// Group by user, each group in the order that user's work should start.
	byUser := map[int64][]Candidate{}
	var users []int64
	for _, c := range queued {
		if c.UserID == 0 {
			// Unattributed: the account is gone, so nobody is waiting for this.
			// The record survives as history (FR-006d); the work does not.
			continue
		}
		if _, seen := byUser[c.UserID]; !seen {
			users = append(users, c.UserID)
		}
		byUser[c.UserID] = append(byUser[c.UserID], c)
	}
	for _, u := range users {
		g := byUser[u]
		sort.SliceStable(g, func(i, j int) bool {
			if g[i].Direct != g[j].Direct {
				return g[i].Direct // a pasted link outranks bulk work
			}
			return g[i].Seq < g[j].Seq
		})
		byUser[u] = g
	}
	// Users are visited in the order their earliest queued work arrived, so
	// rotation is deterministic and does not depend on map iteration.
	sort.SliceStable(users, func(i, j int) bool {
		return byUser[users[i]][0].Seq < byUser[users[j]][0].Seq
	})

	// Round-robin: one from each user in turn, until the slots run out or the
	// queue does.
	var out []Candidate
	for len(out) < slots {
		progressed := false
		for _, u := range users {
			if len(out) >= slots {
				break
			}
			g := byUser[u]
			if len(g) == 0 {
				continue
			}
			out = append(out, g[0])
			byUser[u] = g[1:]
			progressed = true
		}
		if !progressed {
			break // every queue is empty
		}
	}
	return out
}
