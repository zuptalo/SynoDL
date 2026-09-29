package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// How a worker is given the YouTube sign-in (spec 1055).
//
// The cookies are a live Google login, so they may not travel in the worker's Job:
// a Job is readable by anyone who can read Jobs, and the server has no permission
// on Secrets (`30-rbac.yaml` says why, and it is not to change for this). Instead
// the Job carries a GRANT, and a small init step trades it for the cookies over
// the cluster network.
//
// A grant is worthless on its own, and that is the whole design:
//
//   - it is honoured ONCE — a second use finds nothing;
//   - it expires after ten minutes, whether or not it was used;
//   - it names exactly one Job, and the request must come from that Job's own pod
//     address (`status.podIP`), which is a fact only the cluster can attest to;
//   - it is refused outright if the request carries a forwarding header. This
//     endpoint shares a port with the ingress, and anything arriving through the
//     ingress carries one — so nothing from outside the cluster can redeem a grant
//     however it got hold of it. That is also why httpx.clientIP, which trusts
//     X-Forwarded-For for rate limiting, is deliberately NOT used here.
//
// Every refusal is the same empty 404, so the endpoint tells a caller nothing
// about which condition failed. Grants live in memory only: a restart loses them,
// and a download whose grant is lost simply runs anonymously.

const signinGrantTTL = 10 * time.Minute

type signinGrant struct {
	job     string
	expires time.Time
}

type signinGrants struct {
	mu  sync.Mutex
	m   map[[32]byte]signinGrant
	now func() time.Time
}

func newSigninGrants() *signinGrants {
	return &signinGrants{m: map[[32]byte]signinGrant{}, now: time.Now}
}

func grantKey(token string) [32]byte { return sha256.Sum256([]byte(token)) }

// mint returns a fresh single-use grant for a Job. Only its hash is kept.
func (g *signinGrants) mint(job string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw[:])
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for k, v := range g.m { // the table stays as small as the Jobs actually waiting
		if now.After(v.expires) {
			delete(g.m, k)
		}
	}
	g.m[grantKey(token)] = signinGrant{job: job, expires: now.Add(signinGrantTTL)}
	return token, nil
}

// peek reports which Job a live grant belongs to, without spending it.
func (g *signinGrants) peek(token string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.m[grantKey(token)]
	if !ok || g.now().After(v.expires) {
		return "", false
	}
	return v.job, true
}

// consume spends a grant. Exactly one caller can ever get true for a grant.
func (g *signinGrants) consume(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := grantKey(token)
	v, ok := g.m[k]
	if !ok {
		return false
	}
	delete(g.m, k)
	return !g.now().After(v.expires)
}

// revoke drops every grant for a Job. Called when the reconciler sees it end.
func (g *signinGrants) revoke(job string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, v := range g.m {
		if v.job == job {
			delete(g.m, k)
		}
	}
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// handleYtdlSignInFetch answers a worker's init step (contracts/api.md).
func handleYtdlSignInFetch(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deny := func(reason string) {
			// A fixed code and nothing else: never the grant, never the address.
			slog.Info("signin fetch denied", "reason", reason)
			w.WriteHeader(http.StatusNotFound)
		}
		if d.signin == nil || d.Jobs == nil || d.Store == nil {
			deny("unavailable")
			return
		}
		for _, h := range []string{"X-Forwarded-For", "Forwarded", "X-Real-Ip"} {
			if r.Header.Get(h) != "" {
				deny("proxied")
				return
			}
		}
		token := bearer(r)
		if token == "" {
			deny("grant")
			return
		}
		job, ok := d.signin.peek(token)
		if !ok {
			deny("grant")
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		from := net.ParseIP(host)
		pods, err := d.Jobs.ListPods(r.Context(), "job-name="+job)
		if err != nil || from == nil {
			deny("address")
			return
		}
		match := false
		for _, p := range pods {
			if p.Metadata.Labels["job-name"] != job {
				continue
			}
			if ip := net.ParseIP(p.Status.PodIP); ip != nil && ip.Equal(from) {
				match = true
				break
			}
		}
		if !match {
			deny("address")
			return
		}
		plain, present, err := d.Store.OpenYoutubeSignInCookies()
		if err != nil || !present {
			deny("none")
			return
		}
		if !d.signin.consume(token) {
			deny("used")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(plain)
		slog.Info("signin fetch ok", "job", job)
	})
}
