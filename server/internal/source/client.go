package source

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"sort"
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
//
// Headers and Cookies are supplied by the DRIVER, not assembled here from a
// fixed set of field names. Before multiple sources existed the client set one
// provider's auth headers on every request it made; with a second source
// configured that would have sent the first source's credentials to the second
// site. Auth is now the driver's own business and travels only to the host that
// driver is calling.
type Req struct {
	Method  string
	URL     string
	Body    string // form-encoded body for POST (may be empty)
	XHR     bool   // true = API/fetch call; false = top-level page (verify)
	Origin  string // provider-supplied Origin/Referer for XHR calls
	Referer string
	Headers map[string]string // driver-supplied auth/identity headers
	Cookies map[string]string // driver-supplied cookies, joined into one Cookie header
	// ContentType overrides the default form encoding for an XHR body.
	ContentType string
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

	// Identity. Values are header-sanitized so a paste artifact (e.g. a wrapped
	// User-Agent with an embedded newline) can't make http fail the request with a
	// cryptic "invalid header field value".
	r.Header.Set("User-Agent", headerSafe(s.UserAgent))
	// Driver-supplied auth. Nothing here is derived from a field name this package
	// knows: the driver decides what its own site needs, so one source's material
	// can never ride along to another's host.
	for k, v := range req.Headers {
		setIf(r, k, v)
	}
	if c := cookieHeader(req.Cookies); c != "" {
		r.Header.Set("Cookie", c)
	}

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
			r.Header.Set("Content-Type", orDefault(req.ContentType, "application/x-www-form-urlencoded"))
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

// cookieHeader joins driver-supplied cookies into one header value. Keys and
// values are sanitized; an empty value drops the pair rather than emitting a
// malformed "k=" that some origins reject.
func cookieHeader(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	// Sorted so the header is deterministic and tests don't flake on map order.
	keys := make([]string, 0, len(cookies))
	for k := range cookies {
		if headerSafe(cookies[k]) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, headerSafe(k)+"="+headerSafe(cookies[k]))
	}
	return strings.Join(parts, "; ")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func setIf(r *http.Request, key, val string) {
	if v := headerSafe(val); v != "" {
		r.Header.Set(key, v)
	}
}

// headerSafe strips CR/LF and trims surrounding whitespace so pasted session
// values (which often wrap across lines) don't produce an invalid HTTP header.
func headerSafe(v string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "").Replace(v))
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
