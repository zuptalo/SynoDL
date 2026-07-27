// Package nas owns the single stored NAS connection that SynoDL uses on behalf
// of all its users (constitution v2.0.0). It keeps a live DSM session, logging
// in with the stored credentials when the session expires. A 2FA account cannot
// be re-authenticated unattended (a fresh time-based code is required), so the
// manager surfaces ErrNeedsReauth and an admin-driven Reauth entry point.
package nas

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"synodl/server/internal/store"
	"synodl/server/internal/syno"
)

// ErrNotConfigured means the setup wizard hasn't stored a NAS connection yet.
var ErrNotConfigured = errors.New("nas: not configured")

// ErrNeedsReauth means the DSM session expired and the stored account uses 2FA,
// so an admin must supply a fresh code before NAS actions can resume.
var ErrNeedsReauth = errors.New("nas: needs 2FA re-authentication")

// Factory builds a syno.Client for a DSM base URL. Production uses
// syno.NewHTTPClient; tests inject a fake pointed at the mock DSM.
type Factory func(baseURL string, tlsInsecure bool) syno.Client

// Manager keeps the one stored NAS session. Safe for concurrent use.
type Manager struct {
	store   *store.Store
	factory Factory

	mu      sync.Mutex
	client  syno.Client
	baseKey string // fingerprint of the stored connection the client was built for
	sid     string // cached live DSM session id ("" = none)
}

// New builds a Manager over the store, using factory to construct DSM clients.
func New(st *store.Store, factory Factory) *Manager {
	return &Manager{store: st, factory: factory}
}

func baseURL(c *store.OperatorConfig) string {
	// DSM speaks HTTPS on its portal port; nas_tls_verify controls whether we
	// verify the certificate (self-signed NAS ⇒ verify off).
	return fmt.Sprintf("https://%s:%d", c.NASAddress, c.NASPort)
}

// clientLocked returns a client for the current stored config, rebuilding it if
// the connection details changed. Caller holds mu.
func (m *Manager) clientLocked() (syno.Client, *store.OperatorConfig, error) {
	cfg, err := m.store.GetOperatorConfig()
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrNotConfigured
	}
	if err != nil {
		return nil, nil, err
	}
	key := fmt.Sprintf("%s|%v", baseURL(cfg), cfg.NASTLSVerify)
	if m.client == nil || m.baseKey != key {
		m.client = m.factory(baseURL(cfg), !cfg.NASTLSVerify)
		m.baseKey = key
		m.sid = "" // new client ⇒ need a fresh login
	}
	return m.client, cfg, nil
}

// current returns the DSM client for the stored connection plus a valid session
// id, logging in with the stored credentials if there is no live session. A 2FA
// account with no live session returns ErrNeedsReauth. The client and sid are
// returned together because a sid is only valid on the client that created it.
func (m *Manager) current(ctx context.Context) (syno.Client, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	client, cfg, err := m.clientLocked()
	if err != nil {
		return nil, "", err
	}
	if m.sid == "" {
		if cfg.NASUses2FA {
			return nil, "", ErrNeedsReauth
		}
		sid, err := client.Login(ctx, cfg.NASAccount, cfg.NASPassword, "")
		if err != nil {
			return nil, "", err
		}
		m.sid = sid
	}
	return client, m.sid, nil
}

// SID returns a valid DSM session id (see current).
func (m *Manager) SID(ctx context.Context) (string, error) {
	_, sid, err := m.current(ctx)
	return sid, err
}

// Reauth establishes a session using the stored credentials plus a fresh OTP —
// the admin-driven path for a 2FA account whose session expired.
func (m *Manager) Reauth(ctx context.Context, otp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	client, cfg, err := m.clientLocked()
	if err != nil {
		return err
	}
	sid, err := client.Login(ctx, cfg.NASAccount, cfg.NASPassword, otp)
	if err != nil {
		return err
	}
	m.sid = sid
	return nil
}

// Invalidate drops the cached session so the next SID re-logs in (or requires
// reauth for a 2FA account). Call it when the NAS reports the session expired.
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.sid = ""
	m.mu.Unlock()
}

// Do runs fn with the DSM client + a valid sid, transparently re-authenticating
// once if the NAS reports the session expired. For a 2FA account the retry
// surfaces ErrNeedsReauth rather than looping.
func (m *Manager) Do(ctx context.Context, fn func(c syno.Client, sid string) error) error {
	client, sid, err := m.current(ctx)
	if err != nil {
		return err
	}
	err = fn(client, sid)
	if isSessionExpired(err) {
		m.Invalidate()
		client, sid, err2 := m.current(ctx)
		if err2 != nil {
			return err2
		}
		return fn(client, sid)
	}
	return err
}

// VerifyLogin checks explicit connection details + credentials by performing a
// real DSM login WITHOUT persisting anything — used by the setup wizard to
// validate before the connection is stored.
func (m *Manager) VerifyLogin(ctx context.Context, address string, port int, tlsVerify bool, account, password, otp string) error {
	c := m.factory(fmt.Sprintf("https://%s:%d", address, port), !tlsVerify)
	sid, err := c.Login(ctx, account, password, otp)
	if err != nil {
		return err
	}
	_ = c.Logout(ctx, sid)
	return nil
}

func isSessionExpired(err error) bool {
	se := syno.AsError(err)
	return se != nil && se.Kind == syno.KindSession
}
