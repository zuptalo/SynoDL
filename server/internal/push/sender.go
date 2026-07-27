package push

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"synodl/server/internal/store"
)

// Sender delivers encrypted Web Push messages using the instance VAPID keys.
type Sender struct {
	hc    *http.Client
	vapid store.VAPID
}

// NewSender builds a Sender for the given VAPID keypair + subject.
func NewSender(v store.VAPID) *Sender {
	return &Sender{hc: &http.Client{Timeout: 15 * time.Second}, vapid: v}
}

// Send delivers payload to one subscription. gone is true when the push service
// reports the subscription expired (404/410) so the caller prunes it; a nil
// error with gone=false means delivered.
func (s *Sender) Send(ctx context.Context, sub store.Subscription, payload []byte) (gone bool, err error) {
	body, err := Encrypt(sub.P256dh, sub.Auth, payload)
	if err != nil {
		return false, err
	}
	authHdr, err := authorizationHeader(sub.Endpoint, s.vapid.Subject, s.vapid.Public, s.vapid.Private)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400") // keep for a day if the device is offline
	resp, err := s.hc.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return false, nil
	case http.StatusNotFound, http.StatusGone:
		return true, nil // subscription no longer valid — prune it
	default:
		return false, fmt.Errorf("push: %s returned %d", sub.Endpoint, resp.StatusCode)
	}
}
