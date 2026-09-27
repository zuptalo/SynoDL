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
