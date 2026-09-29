package ytdl

import "testing"

// Spec 2035. yt-dlp says exactly why a download failed; the Job only says that
// it did. These are the messages seen from real production failures, and the
// plain sentence each should become.
func TestFailureFromOutput(t *testing.T) {
	cases := []struct {
		name, out, want string
		ok              bool
	}{
		{"age gate", "[youtube] x: Downloading webpage\nERROR: [youtube] qpgTC9MDx1o: Sign in to confirm your age. This video may be inappropriate for some users.", ReasonAgeRestricted, true},
		{"bot check", "ERROR: [youtube] abc: Sign in to confirm you’re not a bot. Use --cookies-from-browser", ReasonRefused, true},
		// Spec 1051: the wording yt-dlp actually printed for vKNZqM0d-xo, which the
		// old "video unavailable" match missed for eight retries.
		{"is unavailable", "[youtube] vKNZqM0d-xo: Downloading webpage\nERROR: [youtube] vKNZqM0d-xo: This video is unavailable", ReasonUnavailable, true},
		{"isn't available anymore", "ERROR: [youtube] abc: This video isn't available anymore", ReasonUnavailable, true},
		{"no longer available", "ERROR: [youtube] abc: This video is no longer available", ReasonUnavailable, true},
		{"403", "[download] 12.0%\nERROR: unable to download video data: HTTP Error 403: Forbidden", ReasonRefused, true},
		{"429", "ERROR: [youtube] abc: HTTP Error 429: Too Many Requests", ReasonRefused, true},
		{"unavailable", "ERROR: [youtube] QMP-o8WXSPM: Video unavailable", ReasonUnavailable, true},
		{"private", "ERROR: [youtube] abc: Private video. Sign in if you've been granted access to this video", ReasonUnavailable, true},
		{"removed", "ERROR: [youtube] abc: Video unavailable. This video has been removed by the uploader", ReasonUnavailable, true},
		{"region", "ERROR: [youtube] abc: Video unavailable. The uploader has not made this video available in your country", ReasonRegion, true},
		{"premium", "ERROR: [youtube] 5LFB3qdmZBM: This video is only available to Music Premium members", ReasonPaid, true},
		{"paid", "ERROR: [youtube] h6Ip3PevVF8: This video requires payment to watch", ReasonPaid, true},
		{"the last error wins", "ERROR: HTTP Error 403: Forbidden\nERROR: [youtube] abc: Video unavailable", ReasonUnavailable, true},
		{"unknown error", "ERROR: something nobody has seen before", "", false},
		{"no error at all", "[download] 100%", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := FailureFromOutput([]byte(c.out))
			if got != c.want || ok != c.ok {
				t.Fatalf("got %q,%v want %q,%v", got, ok, c.want, c.ok)
			}
		})
	}
}

// Spec 1055: only the bot-check is evidence about a saved sign-in.
func TestIsBotCheck(t *testing.T) {
	for raw, want := range map[string]bool{
		"ERROR: [youtube] x: Sign in to confirm you’re not a bot. Use --cookies": true,
		"ERROR: unable to download video data: HTTP Error 403: Forbidden":        false,
		"ERROR: [youtube] x: Video unavailable":                                  false,
		"ERROR: Sign in to confirm you're not a bot\nERROR: HTTP Error 429":      false, // the LAST error decides
		"WARNING: something not a bot related\nERROR: [youtube] x: not a bot":    true,
		"": false,
	} {
		if got := IsBotCheck([]byte(raw)); got != want {
			t.Errorf("IsBotCheck(%q) = %v, want %v", raw, got, want)
		}
	}
}
