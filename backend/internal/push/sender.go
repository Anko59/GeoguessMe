package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// vapidTTL caps the JWT validity window. RFC 8292 allows up to 24h; 12h is the
// conventional application-server value and avoids clock-skew rejections.
const vapidTTL = 12 * time.Hour

// pushTTL is the "TTL" header (seconds) sent on every push request. It tells
// the push service how long to retain an undeliverable message. Four weeks is
// the maximum; we use 24h since notifications are time-sensitive.
const pushTTL = "86400"

// ErrSubscriptionGone indicates a permanent delivery failure (410 Gone, 404 Not
// Found, or a malformed subscription). Callers MUST remove the subscription so
// dead endpoints are never retried indefinitely.
var ErrSubscriptionGone = errors.New("push subscription is no longer valid")

// HTTPDoer is the minimal HTTP client surface the sender depends on. http.Client
// satisfies it; tests inject an httptest.Server-backed client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Sender encrypts and delivers push messages to a single push service endpoint
// using VAPID (RFC 8292) authentication. It holds no per-subscription state.
// When a guard is provided, Send refuses endpoints outside the provider
// allowlist and the default client goes through the hardened transport.
type Sender struct {
	keys    *KeyPair
	subject string
	client  HTTPDoer
	guard   *EndpointGuard
	now     func() time.Time
}

// NewSender constructs a Sender. guard may be nil to keep legacy validation
// (used by unit tests); when non-nil, Send rejects endpoints the guard does
// not allow and a nil client resolves to the guard's hardened client. A client
// passed explicitly bypasses the guarded transport but never the allowlist.
func NewSender(keys *KeyPair, subject string, guard *EndpointGuard, client HTTPDoer) *Sender {
	if client == nil {
		if guard != nil {
			client = guard.Client()
		} else {
			client = &http.Client{Timeout: 10 * time.Second}
		}
	}
	return &Sender{keys: keys, subject: subject, client: client, guard: guard, now: time.Now}
}

// Send encrypts payload for the subscription's credentials and POSTs it to the
// subscription endpoint. A non-nil error that wraps ErrSubscriptionGone means
// the subscription must be deleted; any other error is transient/retryable and
// leaves the subscription intact.
func (s *Sender) Send(ctx context.Context, sub *Subscription, payload []byte) error {
	// The endpoint host is validated against the provider allowlist before any
	// crypto or network work. A subscription outside the allowlist is permanent
	// invalid: it must be dropped, never retried against an arbitrary host.
	if s.guard != nil {
		if err := s.guard.ValidateEndpoint(sub.Endpoint); err != nil {
			return fmt.Errorf("%w: %v", ErrSubscriptionGone, err)
		}
	}
	receiverBytes, err := decodeReceiverKey(sub.P256DH)
	if err != nil || len(receiverBytes) != 65 || receiverBytes[0] != 0x04 {
		return fmt.Errorf("%w: invalid p256dh key", ErrSubscriptionGone)
	}
	receiver, err := ecdh.P256().NewPublicKey(receiverBytes)
	if err != nil {
		return fmt.Errorf("%w: invalid p256dh key", ErrSubscriptionGone)
	}
	auth, err := base64.RawURLEncoding.DecodeString(sub.Auth)
	if err != nil || len(auth) != 16 {
		return fmt.Errorf("%w: invalid auth secret", ErrSubscriptionGone)
	}
	eph, err := generateEphemeralKeys()
	if err != nil {
		return err
	}
	message, err := encryptMessage(payload, receiver, auth, eph)
	if err != nil {
		return err
	}
	token, err := s.vapidJWT(sub.Endpoint)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(message.bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", token)
	req.Header.Set("TTL", pushTTL)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("push request failed: %w", err)
	}
	statusErr := classifyStatus(resp)
	if closeErr := resp.Body.Close(); statusErr == nil && closeErr != nil {
		return fmt.Errorf("close push response: %w", closeErr)
	}
	return statusErr
}

// classifyStatus maps push-service response codes to permanent vs. transient
// outcomes. Permanent failures wrap ErrSubscriptionGone; everything else is a
// retryable delivery error.
func classifyStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusGone:
		return fmt.Errorf("%w: endpoint returned %d", ErrSubscriptionGone, resp.StatusCode)
	case resp.StatusCode >= 400 && resp.StatusCode < 500 &&
		resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusRequestTimeout:
		return fmt.Errorf("%w: endpoint returned %d", ErrSubscriptionGone, resp.StatusCode)
	default:
		return fmt.Errorf("transient push failure: endpoint returned %d", resp.StatusCode)
	}
}

// vapidJWT builds the "vapid t=<jwt>, k=<key>" Authorization value per RFC 8292.
func (s *Sender) vapidJWT(endpoint string) (string, error) {
	origin, err := endpointOrigin(endpoint)
	if err != nil {
		return "", err
	}
	exp := s.now().Add(vapidTTL).Unix()
	claims := struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{Aud: origin, Exp: exp, Sub: s.subject}
	header := map[string]string{"alg": "ES256", "typ": "JWT"}
	token, err := signES256JWT(s.keys.signer(), header, claims)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("vapid t=%s, k=%s", token, s.keys.PublicKeyBase64URL()), nil
}

// endpointOrigin returns scheme://host[:port] for the push endpoint so it can be
// used as the VAPID "aud" claim.
func endpointOrigin(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid push endpoint: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("push endpoint must use http(s), got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("push endpoint is missing a host")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// signES256JWT produces a compact JWT signed with raw ECDSA P-256 SHA-256.
func signES256JWT(signer *ecdsa.PrivateKey, header, claims any) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, signer, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign vapid jwt: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// decodeReceiverKey parses the base64url p256dh subscription key into raw bytes.
func decodeReceiverKey(p256dh string) ([]byte, error) {
	// Browsers sometimes emit padding; tolerate it by stripping before RawURL.
	value := strings.TrimRight(p256dh, "=")
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid p256dh key: %w", err)
	}
	return decoded, nil
}
