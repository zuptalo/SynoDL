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

	ReasonRefused = "YouTube turned the request away — trying again later usually works"
	// ReasonRefusedRetrying is a refusal SynoDL will retry by itself once the
	// cool-down has passed (spec 1043). Its own text so the row says what is
	// going to happen rather than asking somebody to do it.
	ReasonRefusedRetrying = "YouTube turned the request away — trying again automatically"
	ReasonUnavailable     = "this video is no longer available on YouTube"
	ReasonAgeRestricted   = "YouTube only plays this to signed-in adults"
	ReasonRegion          = "YouTube does not offer this video in this region"
	ReasonPaid            = "YouTube only plays this to paying members"
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
	// YouTube words a removal several ways, and yt-dlp passes the wording
	// through: "Video unavailable", "This video is unavailable", "This video
	// isn't available anymore", "This video is no longer available". A track in
	// production sat on "did not complete" through eight retries because only
	// the first form was known (spec 1051) — so the match is on the two words
	// that every form shares, in either order, rather than on one phrase.
	case has("video unavailable", "video is unavailable", "isn't available", "is not available",
		"no longer available", "not available anymore", "private video", "has been removed",
		"account associated with this video has been terminated"):
		return ReasonUnavailable, true
	}
	return "", false
}
