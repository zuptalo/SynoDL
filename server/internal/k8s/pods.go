package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Pods: the two calls spec 0013 added, and deliberately only two.
//
// Progress, and the fact that a lyrics file was written in a particular
// language, exist NOWHERE except in the worker's own output. The Job object
// carries none of it, and the server never mounts the media library, so it
// cannot look at what the worker produced either. Reading the worker's output
// is the only route to those facts that does not require building our own
// worker image around the pinned upstream one and handing it a credential.
//
// Constitution v2.2.0 admits this as part of what orchestration needs, and is
// explicit that it is NARROWER than controlling a worker: same namespace, only
// workloads SynoDL created, and never a foothold for arguing exec or attach.
// The RBAC reflects that — `pods/log` with verb `get`, and nothing else.
//
// Listing pods needs no new permission at all: `pods: get, list` has been
// granted since spec 0012, so a Job's state could be reported accurately.

// maxPodLogBytes bounds a single log read (FR-013e).
//
// A worker's output is small — a few hundred progress lines — but it is
// produced by a third-party program fetching from a third-party site, so its
// size is not ours to assume. This is the same instinct as the 4 MiB cap on
// control-plane responses: read what could plausibly be needed, and drop the
// rest rather than allocate it.
const maxPodLogBytes = 256 << 10

// PodLogOptions is the subset of the log endpoint's query we use.
type PodLogOptions struct {
	// Container names which container's output to read. A worker pod has one,
	// but naming it means a future sidecar could not silently change what is read.
	Container string
	// TailLines bounds the read at the source as well as at the reader. Progress
	// is in the most recent lines; the beginning of a long run is of no interest.
	TailLines int
}

func (c *Client) podsURL(suffix string, q url.Values) string {
	u := c.cfg.Host + "/api/v1/namespaces/" + url.PathEscape(c.cfg.Namespace) + "/pods"
	if suffix != "" {
		u += "/" + suffix
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// ListPods returns every pod matching the label selector, in SynoDL's namespace.
//
// The selector is what keeps this from ever seeing a pod SynoDL did not create:
// callers pass the same ManagedBy + Kind selector the Jobs list uses.
func (c *Client) ListPods(ctx context.Context, selector string) ([]Pod, error) {
	q := url.Values{}
	if selector != "" {
		q.Set("labelSelector", selector)
	}
	var out PodList
	if err := c.do(ctx, http.MethodGet, c.podsURL("", q), nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// PodLog returns one pod's output, bounded.
//
// It does not reuse do(): that decodes JSON, and this endpoint answers
// text/plain. The error shape is kept identical so IsNotFound and IsForbidden
// work here exactly as they do for Jobs — a swept pod is 404 and an ordinary
// outcome, while 403 in practice means the Role is missing `pods/log`.
func (c *Client) PodLog(ctx context.Context, name string, opts PodLogOptions) ([]byte, error) {
	q := url.Values{}
	if opts.Container != "" {
		q.Set("container", opts.Container)
	}
	if opts.TailLines > 0 {
		q.Set("tailLines", strconv.Itoa(opts.TailLines))
	}
	// Never `follow`: a streaming read would hold a connection open for the
	// life of a download, and the reconciler polls precisely so it does not.

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.podsURL(url.PathEscape(name)+"/log", q), nil)
	if err != nil {
		return nil, fmt.Errorf("k8s: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	// Deliberately NO Accept header.
	//
	// The response body is plain text, so `Accept: text/plain` is the obvious
	// thing to send — and the API server answers 406 for it:
	//
	//	only the following media types are accepted: application/json,
	//	application/yaml, application/vnd.kubernetes.protobuf
	//
	// Content negotiation is on the API's own media types, not on the body this
	// particular subresource happens to return. kubectl sends no text/plain
	// either. Sending nothing lets the server return the log as it always does.

	resp, err := c.http.Do(req)
	if err != nil {
		// The pod name is included; the token is not, and it lives only in a header.
		return nil, fmt.Errorf("k8s: pod log: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPodLogBytes))
	if err != nil {
		return nil, fmt.Errorf("k8s: read pod log: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// An error body here IS JSON even though success is not, so the same
		// APIError shape as every other call is available.
		var status struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &status)
		return nil, &APIError{Status: resp.StatusCode, Message: status.Message}
	}
	return raw, nil
}
