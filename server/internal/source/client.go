package source

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the shared outbound HTTP client for provider calls. It speaks HTTP/2
// (required — the provider's bot protection rejects HTTP/1.1 even with a valid
// clearance cookie) using pure Go stdlib: no headless browser, no third-party TLS
// library. Every request is checked against the provider's host allowlist.
type Client struct {
	http *http.Client
}

// NewClient builds the outbound client. ForceAttemptHTTP2 is essential: a custom
// TLSClientConfig otherwise disables Go's automatic HTTP/2, which downgrades to
// HTTP/1.1 and gets challenged.
func NewClient() *Client {
	return &Client{http: &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			TLSClientConfig:     &tls.Config{},
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}}
}

// Req describes one outbound call.
type Req struct {
	Method  string
	URL     string
	Body    string // form-encoded body for POST (may be empty)
	XHR     bool   // true = API/fetch call; false = top-level page (verify)
	Origin  string // provider-supplied Origin/Referer for XHR calls
	Referer string
}

// Resp is a minimal response view.
type Resp struct {
	Status int
	Body   []byte
}

// Do performs req against the provider using the session material, after
// verifying the target host is allowed. It maps a bot-protection challenge to
// *ErrNeedsRefresh{clearance}; token/JSON semantics are the driver's job.
func (c *Client) Do(ctx context.Context, s Session, allowHosts []string, req Req) (*Resp, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	if !HostAllowed(u.Hostname(), allowHosts) {
		return nil, ErrHostNotAllowed
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	r, err := http.NewRequestWithContext(ctx, method, req.URL, body)
	if err != nil {
		return nil, err
	}

	// Identity + bot-protection clearance.
	r.Header.Set("User-Agent", s.UserAgent)
	if s.CFClearance != "" {
		r.Header.Set("Cookie", "cf_clearance="+s.CFClearance)
	}
	// Provider API auth headers (harmless on page loads).
	setIf(r, "c-api-key", s.CAPIKey)
	setIf(r, "c-token", s.CToken)
	setIf(r, "c-platform", s.CPlatform)
	setIf(r, "c-app-version", s.CAppVersion)
	setIf(r, "c-useragent", s.UserAgent)

	// Client-hints / fetch metadata so we don't stand out as a bot.
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.Header.Set("Sec-Ch-Ua", `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`)
	r.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	r.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	if req.XHR {
		r.Header.Set("Accept", "application/json, text/plain, */*")
		r.Header.Set("Sec-Fetch-Dest", "empty")
		r.Header.Set("Sec-Fetch-Mode", "cors")
		r.Header.Set("Sec-Fetch-Site", "same-site")
		r.Header.Set("X-Requested-With", "XMLHttpRequest")
		setIf(r, "Origin", req.Origin)
		setIf(r, "Referer", req.Referer)
		if req.Body != "" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Header.Set("Sec-Fetch-Dest", "document")
		r.Header.Set("Sec-Fetch-Mode", "navigate")
		r.Header.Set("Sec-Fetch-Site", "none")
		r.Header.Set("Upgrade-Insecure-Requests", "1")
	}

	resp, err := c.http.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

	// Bot-protection challenge → the clearance cookie has expired (or the IP no
	// longer matches). Detect before returning the body to any caller.
	if resp.Header.Get("CF-Mitigated") == "challenge" || looksLikeChallenge(b) {
		return nil, &ErrNeedsRefresh{Layer: LayerClearance}
	}
	return &Resp{Status: resp.StatusCode, Body: b}, nil
}

// HostAllowed reports whether host is in the allowlist. A match is exact or a
// subdomain suffix of an allowed entry (so "eu-download-11.example.info" matches
// an allowed "example.info"). This resolves the download-host match semantics
// (analyze finding A1 / checklist CHK016).
func HostAllowed(host string, allow []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}

func setIf(r *http.Request, key, val string) {
	if val != "" {
		r.Header.Set(key, val)
	}
}

func looksLikeChallenge(b []byte) bool {
	s := string(b)
	for _, m := range []string{"Just a moment", "cf-chl", "__cf_chl", "challenge-platform", "Attention Required"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
