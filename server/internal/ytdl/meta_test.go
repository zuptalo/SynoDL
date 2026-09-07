package ytdl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func describeAgainst(t *testing.T, h http.HandlerFunc, raw string) Description {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	target, err := Classify(raw)
	if err != nil {
		t.Fatalf("Classify(%q): %v", raw, err)
	}
	return Describer{BaseURL: srv.URL, Timeout: 2 * time.Second}.Describe(context.Background(), target)
}

func TestDescribe_ReadsTitleUploaderAndArtwork(t *testing.T) {
	var gotQuery string
	got := describeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{
			"title": "Googoosh - Hastamo Nistam",
			"author_name": "Googoosh Official",
			"thumbnail_url": "https://i.ytimg.com/vi/abc/hqdefault.jpg"
		}`))
	}, "https://youtu.be/abc")

	if got.Title != "Googoosh - Hastamo Nistam" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Uploader != "Googoosh Official" {
		t.Errorf("Uploader = %q", got.Uploader)
	}
	if got.Artwork != "https://i.ytimg.com/vi/abc/hqdefault.jpg" {
		t.Errorf("Artwork = %q", got.Artwork)
	}
	// The NORMALISED url is what gets asked about, not the user's raw text.
	if !strings.Contains(gotQuery, "format=json") || !strings.Contains(gotQuery, "youtu.be") {
		t.Errorf("query = %q, want the normalised url and format=json", gotQuery)
	}
}

// Every one of these is a normal Tuesday for a third-party endpoint, and none
// of them may cost the user their download (FR-003).
func TestDescribe_FailsSilently(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"not found", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }},
		{"server error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }},
		{"garbage body", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"title": not json`))
		}},
		{"empty body", func(w http.ResponseWriter, r *http.Request) {}},
		{"html instead of json", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<!doctype html><title>nope</title>`))
		}},
		{"absurd body", func(w http.ResponseWriter, r *http.Request) {
			// A response far past any sane oEmbed payload must not be swallowed
			// whole into memory.
			_, _ = w.Write([]byte(`{"title":"` + strings.Repeat("A", 4<<20) + `"}`))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := describeAgainst(t, tc.h, "https://youtu.be/abc")
			if got.Title != "" || got.Uploader != "" || got.Artwork != "" {
				t.Errorf("want an empty description, got %+v", got)
			}
		})
	}
}

// The deadline is what protects submission latency (FR-002, SC-003).
func TestDescribe_GivesUpQuickly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		_, _ = w.Write([]byte(`{"title":"too late"}`))
	}))
	t.Cleanup(srv.Close)
	target, _ := Classify("https://youtu.be/abc")

	start := time.Now()
	got := Describer{BaseURL: srv.URL, Timeout: 300 * time.Millisecond}.
		Describe(context.Background(), target)
	elapsed := time.Since(start)

	if got.Title != "" {
		t.Errorf("a timed-out lookup must yield nothing, got %+v", got)
	}
	if elapsed > 2*time.Second {
		t.Errorf("waited %v; the deadline is what keeps submission fast", elapsed)
	}
}

// FR-006: a playlist row must not be labelled with one track's name, which
// would misdescribe what is actually being fetched.
func TestDescribe_PlaylistNamesTheCollection(t *testing.T) {
	got := describeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"title":"Best of 2026","author_name":"Someone"}`))
	}, "https://youtube.com/playlist?list=PL1")
	if got.Title != "Best of 2026" {
		t.Errorf("Title = %q, want the playlist's own name", got.Title)
	}
}

// A channel has no oEmbed document at all; the caller falls back to the link.
func TestDescribe_ChannelYieldsNothingRatherThanGuessing(t *testing.T) {
	got := describeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}, "https://youtube.com/@someartist")
	if got.Title != "" {
		t.Errorf("want nothing rather than a guess, got %+v", got)
	}
}

func TestDescribe_TrimsAndIgnoresBlanks(t *testing.T) {
	got := describeAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"title":"  Spaced  ","author_name":"   ","thumbnail_url":""}`))
	}, "https://youtu.be/abc")
	if got.Title != "Spaced" {
		t.Errorf("Title = %q, want it trimmed", got.Title)
	}
	if got.Uploader != "" {
		t.Errorf("a whitespace-only uploader is not an uploader, got %q", got.Uploader)
	}
}
