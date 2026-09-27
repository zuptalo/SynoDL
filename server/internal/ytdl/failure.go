package ytdl

import "strings"

// Why a download failed, in the words a person reads (spec 2035).
//
// The Job only knows THAT its worker exited non-zero, so every failure used to
// read "the download did not complete" — which made a video that no longer
// exists look the same as YouTube refusing a request for a minute, when the
// first is final and the second is worth retrying. yt-dlp states the cause on
// an `ERROR:` line; this turns the ones that matter into plain sentences.
//
// Matching is on yt-dlp's message text, which is not a stable interface. An
// unrecognised message is therefore not guessed at: it reports ok=false and the
// caller keeps the generic reason.
const (
	// ReasonGeneric is all a Job's own status can say. It is what a failure
	// reads when the worker's output is gone or unrecognised.
	ReasonGeneric = "the download did not complete"

	ReasonRefused       = "YouTube turned the request away — trying again later usually works"
	ReasonUnavailable   = "this video is no longer available on YouTube"
	ReasonAgeRestricted = "YouTube only plays this to signed-in adults"
	ReasonRegion        = "YouTube does not offer this video in this region"
	ReasonPaid          = "YouTube only plays this to paying members"
)

// FailureFromOutput reads the LAST error a worker printed and names it.
func FailureFromOutput(raw []byte) (string, bool) {
	last := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ERROR:") {
			last = line
		}
	}
	if last == "" {
		return "", false
	}
	msg := strings.ToLower(last)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(msg, s) {
				return true
			}
		}
		return false
	}
	// Order matters: a region block and a removal both arrive as "Video
	// unavailable. …", so the specific cause is checked before the general one.
	switch {
	case has("confirm your age", "age-restricted", "inappropriate for some users"):
		return ReasonAgeRestricted, true
	case has("premium members", "requires payment", "members-only", "join this channel"):
		return ReasonPaid, true
	case has("not a bot", "http error 403", "http error 429", "too many requests"):
		return ReasonRefused, true
	case has("in your country", "from your location"):
		return ReasonRegion, true
	case has("video unavailable", "private video", "has been removed", "account associated with this video has been terminated"):
		return ReasonUnavailable, true
	}
	return "", false
}
