package ytdl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Description is what could be learned about a request: what it is, who
// published it, and where its artwork lives.
//
// Every field is best-effort and empty more often than a database column would
// be. That is the contract, not a shortcoming — the caller renders what it has
// and falls back to the link for what it does not.
type Description struct {
	Title    string
	Uploader string
	Artwork  string
}

// Empty reports whether nothing at all was learned.
func (d Description) Empty() bool { return d.Title == "" && d.Uploader == "" && d.Artwork == "" }

const (
	// oEmbed is public, unauthenticated, key-less and quota-less. It is asked
	// about a link that has ALREADY passed the host allowlist, so this cannot be
	// used to make the server fetch somewhere of a caller's choosing.
	defaultOEmbedURL = "https://www.youtube.com/oembed"

	// Short on purpose. This runs inside request acceptance, so the deadline is
	// what keeps submitting a download feeling instant when the endpoint is slow
	// (FR-002). Missing a title costs a nicer row; a slow submit costs the user.
	defaultDescribeTimeout = 3 * time.Second

	// An oEmbed document is a few hundred bytes. Anything past this is not one,
	// and reading it would be the only unbounded allocation in the path.
	maxDescribeBytes = 64 << 10
)

// Describer looks up what a link is. The zero value works.
type Describer struct {
	Client  *http.Client
	BaseURL string
	Timeout time.Duration
}

// Describe returns what could be learned, and an empty Description when
// anything at all goes wrong.
//
// It deliberately returns no error. Every failure here — unreachable, 404,
// garbage, slow — means the same thing to the only caller: render the link
// instead. Returning an error would invite someone to fail the download over
// it, which FR-003 forbids.
func (d Describer) Describe(ctx context.Context, t Target) Description {
	base := d.BaseURL
	if base == "" {
		base = defaultOEmbedURL
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = defaultDescribeTimeout
	}
	client := d.Client
	if client == nil {
		client = &http.Client{}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	q := url.Values{"url": {t.URL}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return Description{}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Description{}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// A channel has no oEmbed document at all, so 404 is an ordinary
		// outcome here rather than a fault worth reporting.
		return Description{}
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDescribeBytes))
	if err != nil {
		return Description{}
	}

	var doc struct {
		Title     string `json:"title"`
		Author    string `json:"author_name"`
		Thumbnail string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		// Includes the truncated-because-absurd case: a body past the cap is
		// cut mid-token and simply fails to parse, which is the right outcome.
		return Description{}
	}

	return Description{
		Title:    strings.TrimSpace(doc.Title),
		Uploader: strings.TrimSpace(doc.Author),
		Artwork:  strings.TrimSpace(doc.Thumbnail),
	}
}
