// Package ytsignin turns whatever an admin pastes or uploads into the cookies
// file YouTube's extractor reads (spec 1055).
//
// The value handled here is a LIVE LOGIN for a Google account. Everything in this
// package is written so that nothing about a cookie's content can leave through
// an error, a log line or a return value the caller might log: errors carry
// counts and fixed codes, and the only text that comes back is the canonical
// cookies file itself, which the caller seals immediately.
package ytsignin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Bounds. A real browser Cookie header is 3–6 KiB; these leave room without
// letting a paste become an allocation.
const (
	MaxPasteBytes = 64 << 10
	MaxCookies    = 200
	maxNameLen    = 128
	maxValueLen   = 4096
	// MinCookies is the fewest that can plausibly be a browser session. One or two
	// cookies is somebody pasting a single value, which cannot sign anything in.
	MinCookies = 3
)

// LoginCookies are the ones that only exist when the browser is signed in. Their
// absence is a warning, not an error: it usually means the header was copied from
// a request that was not signed in, but SynoDL cannot be sure and a session that
// works is worth more than a tidy refusal.
var LoginCookies = []string{"SAPISID", "__Secure-3PSID", "LOGIN_INFO", "SID"}

// Errors. Each is a fixed code; none can contain pasted text.
var (
	ErrTooLarge     = errors.New("too_large")
	ErrUnrecognised = errors.New("unrecognised")
	ErrTooFew       = errors.New("too_few_cookies")
	ErrInvalid      = errors.New("invalid")
)

// Result is what a successful parse yields.
type Result struct {
	// Netscape is the canonical cookies file, ending in a newline.
	Netscape []byte
	Count    int
	// Found and Missing partition LoginCookies.
	Found, Missing []string
}

// Warning is "no_login_cookies" when none of the login cookies are present.
func (r Result) Warning() string {
	if len(r.Found) == 0 {
		return "no_login_cookies"
	}
	return ""
}

type cookie struct {
	domain, path, name, value string
	secure                    bool
	expiry                    string
	httpOnly                  bool
}

// Parse accepts a browser Cookie request header (with or without the "Cookie:"
// prefix), or a cookies file in the tab-separated Netscape format, and returns the
// canonical file. Only YouTube's and Google's own cookies are kept.
func Parse(text string) (Result, error) {
	if len(text) > MaxPasteBytes {
		return Result{}, ErrTooLarge
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "\uFEFF"))
	if text == "" {
		return Result{}, ErrUnrecognised
	}
	var cs []cookie
	var err error
	if looksNetscape(text) {
		cs, err = parseNetscape(text)
	} else {
		cs, err = parseHeader(text)
	}
	if err != nil {
		return Result{}, err
	}
	if len(cs) > MaxCookies {
		return Result{}, ErrTooLarge
	}
	if len(cs) < MinCookies {
		return Result{}, ErrTooFew
	}
	return build(cs), nil
}

func looksNetscape(text string) bool {
	if strings.HasPrefix(text, "# Netscape HTTP Cookie File") || strings.HasPrefix(text, "# HTTP Cookie File") {
		return true
	}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || (strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "#HttpOnly_")) {
			continue
		}
		return strings.Count(l, "\t") >= 6
	}
	return false
}

func parseNetscape(text string) ([]cookie, error) {
	var out []cookie
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		httpOnly := false
		if strings.HasPrefix(l, "#HttpOnly_") {
			httpOnly = true
			l = strings.TrimPrefix(l, "#HttpOnly_")
		} else if strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) != 7 {
			return nil, ErrInvalid
		}
		c := cookie{domain: strings.ToLower(strings.TrimSpace(f[0])), path: f[2], secure: strings.EqualFold(f[3], "TRUE"),
			expiry: f[4], name: f[5], value: f[6], httpOnly: httpOnly}
		if !kept(c.domain) {
			continue
		}
		if !valid(c.name, c.value) || c.path == "" || !strings.HasPrefix(c.path, "/") {
			return nil, ErrInvalid
		}
		out = append(out, c)
	}
	return out, nil
}

// secureNames get the Secure flag on a header paste, where the browser has not
// said. The extractor ignores the flag for cookies it is sending to https, so a
// wrong guess costs nothing; the list only makes the file look like the browser's.
var secureNames = map[string]bool{"SAPISID": true, "SSID": true, "APISID": true, "SID": true, "HSID": true, "LOGIN_INFO": true}

func parseHeader(text string) ([]cookie, error) {
	// A header may arrive as "Cookie: a=1; b=2", possibly wrapped over lines.
	t := strings.TrimSpace(text)
	if i := strings.IndexByte(t, ':'); i > 0 && strings.EqualFold(strings.TrimSpace(t[:i]), "cookie") {
		t = t[i+1:]
	}
	t = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(t)
	var out []cookie
	for _, part := range strings.Split(t, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, ErrUnrecognised
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !valid(name, value) {
			return nil, ErrInvalid
		}
		out = append(out, cookie{domain: ".youtube.com", path: "/", name: name, value: value, expiry: "2000000000",
			secure: strings.HasPrefix(name, "__Secure-") || strings.HasPrefix(name, "__Host-") || secureNames[name]})
	}
	return out, nil
}

func valid(name, value string) bool {
	if name == "" || len(name) > maxNameLen || len(value) > maxValueLen {
		return false
	}
	for _, s := range []string{name, value} {
		for _, r := range s {
			if r < 0x20 || r == 0x7f || r == '\t' {
				return false
			}
		}
	}
	return !strings.ContainsAny(name, " ;=")
}

// kept reports whether a cookie domain is one of YouTube's or Google's own.
func kept(domain string) bool {
	d := strings.TrimPrefix(domain, ".")
	for _, root := range []string{"youtube.com", "google.com"} {
		if d == root || strings.HasSuffix(d, "."+root) {
			return true
		}
	}
	return false
}

func build(cs []cookie) Result {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	names := map[string]bool{}
	n := 0
	for _, c := range cs {
		dom := c.domain
		if !strings.HasPrefix(dom, ".") {
			dom = "." + dom
		}
		if c.httpOnly {
			b.WriteString("#HttpOnly_")
		}
		sec := "FALSE"
		if c.secure {
			sec = "TRUE"
		}
		exp := c.expiry
		if exp == "" {
			exp = "0"
		}
		fmt.Fprintf(&b, "%s\tTRUE\t%s\t%s\t%s\t%s\t%s\n", dom, c.path, sec, exp, c.name, c.value)
		names[c.name] = true
		n++
	}
	var found, missing []string
	for _, l := range LoginCookies {
		if names[l] {
			found = append(found, l)
		} else {
			missing = append(missing, l)
		}
	}
	sort.Strings(found)
	sort.Strings(missing)
	return Result{Netscape: []byte(b.String()), Count: n, Found: found, Missing: missing}
}
