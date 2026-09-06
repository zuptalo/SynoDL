package k8s

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to ONE namespace and knows only about Jobs.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds a client. TLS is pinned to the cluster CA when one is supplied;
// a CA-less plain-HTTP host is allowed so the same code runs against the
// in-repo mock orchestrator in dev and e2e.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("k8s: no API server host")
	}
	if strings.TrimSpace(cfg.Namespace) == "" {
		return nil, errors.New("k8s: no namespace")
	}

	tr := &http.Transport{
		MaxIdleConns:        4,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if strings.HasPrefix(cfg.Host, "https://") && len(cfg.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CACert) {
			return nil, errors.New("k8s: cluster CA is not valid PEM")
		}
		// Verification stays ON. Unlike the NAS connection there is no operator
		// opt-out here: the cluster always presents a cert its own CA signed.
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}

	return &Client{
		cfg: cfg,
		// A generous but finite timeout: these are small control-plane calls,
		// never the download itself.
		http: &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}, nil
}

// Namespace reports the namespace this client is confined to.
func (c *Client) Namespace() string { return c.cfg.Namespace }

// APIError is a non-2xx response. It carries the status and the API server's
// own message — and deliberately nothing else, so a credential can never travel
// inside it.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("kubernetes api: status %d", e.Status)
	}
	return fmt.Sprintf("kubernetes api: status %d: %s", e.Status, e.Message)
}

func statusIs(err error, code int) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == code
}

// IsConflict reports a 409 — for a create, "this name already exists".
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsForbidden reports a 403, which in practice means the Role is missing a verb.
func IsForbidden(err error) bool { return statusIs(err, http.StatusForbidden) }

func (c *Client) jobsURL(suffix string, q url.Values) string {
	u := c.cfg.Host + "/apis/batch/v1/namespaces/" + url.PathEscape(c.cfg.Namespace) + "/jobs"
	if suffix != "" {
		u += "/" + url.PathEscape(suffix)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// do performs one API call and decodes the body into out (when out != nil).
func (c *Client) do(ctx context.Context, method, url string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("k8s: encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return fmt.Errorf("k8s: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// The URL is included; the token is not, and it lives only in a header.
		return fmt.Errorf("k8s: %s: %w", method, err)
	}
	defer resp.Body.Close()

	// Bounded read: a control-plane response is small, and this stops a
	// misbehaving endpoint from becoming a memory problem.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("k8s: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var status struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &status)
		return &APIError{Status: resp.StatusCode, Message: status.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("k8s: decode response: %w", err)
	}
	return nil
}

// CreateJob submits a Job.
func (c *Client) CreateJob(ctx context.Context, j *Job) (*Job, error) {
	var out Job
	if err := c.do(ctx, http.MethodPost, c.jobsURL("", nil), j, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListJobs returns every Job matching the label selector.
//
// This is the single call behind the whole task list: state is derived from it
// rather than mirrored into the database, which is why a server restart cannot
// desynchronise anything (FR-019).
func (c *Client) ListJobs(ctx context.Context, selector string) ([]Job, error) {
	q := url.Values{}
	if selector != "" {
		q.Set("labelSelector", selector)
	}
	var out JobList
	if err := c.do(ctx, http.MethodGet, c.jobsURL("", q), nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// DeleteJob removes a Job and the pod it created.
//
// An absent Job is success: TTL sweeps finished Jobs on its own, so "already
// gone" is the ordinary case rather than a fault.
func (c *Client) DeleteJob(ctx context.Context, name string) error {
	q := url.Values{}
	// Without Background propagation the Job goes and its pod is orphaned.
	q.Set("propagationPolicy", "Background")
	err := c.do(ctx, http.MethodDelete, c.jobsURL(name, q), nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}
