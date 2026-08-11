package push

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/internal/config"
)

// stubResolver maps hostnames to fixed IP addresses so the dial guard can be
// exercised without real DNS.
type stubResolver map[string][]net.IP

func (r stubResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, ip := range r[host] {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

// recordDial records the address each dial would connect to and fails the dial
// so tests never touch the network.
func recordDial(dialed *[]string) dialFunc {
	return func(_ context.Context, _, addr string) (net.Conn, error) {
		*dialed = append(*dialed, addr)
		return nil, errors.New("dial stub called")
	}
}

// flippingResolver returns a different answer on every call, modeling DNS
// rebinding across sends.
type flippingResolver struct {
	answers [][]net.IP
	i       int
}

func (f *flippingResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	answer := f.answers[f.i]
	if f.i < len(f.answers)-1 {
		f.i++
	}
	out := make([]net.IPAddr, 0, len(answer))
	for _, ip := range answer {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

// cannedDoer returns a fixed status without any network, letting sender tests
// verify endpoint validation while avoiding real outbound connections.
type cannedDoer struct {
	status int
	called *bool
}

func (c cannedDoer) Do(*http.Request) (*http.Response, error) {
	if c.called != nil {
		*c.called = true
	}
	return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

func TestValidateEndpointAgainstAllowlist(t *testing.T) {
	guard := newEndpointGuard([]string{"fcm.googleapis.com", "push.services.mozilla.com", "web-push.apple.com", "wns.windows.com"}, false, stubResolver{}, recordDial(&[]string{}))
	cases := []struct {
		name     string
		endpoint string
		wantOK   bool
	}{
		{"fcm exact", "https://fcm.googleapis.com/fcm/send/abc", true},
		{"mozilla exact", "https://push.services.mozilla.com/v1/abc", true},
		{"apple exact", "https://web-push.apple.com/abc", true},
		{"wns exact", "https://wns.windows.com/abc", true},
		{"allowlisted subdomain", "https://eu.fcm.googleapis.com/abc", true},
		{"unlisted domain", "https://evil.example.com/abc", false},
		{"suffix confusion", "https://fcm.googleapis.com.evil.example/abc", false},
		{"loopback without permission", "https://127.0.0.1:9999/abc", false},
		{"plain http", "http://fcm.googleapis.com/abc", false},
		{"nonstandard HTTPS port", "https://fcm.googleapis.com:8443/abc", false},
		{"ftp scheme", "ftp://fcm.googleapis.com/abc", false},
		{"relative url", "/abc", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := guard.ValidateEndpoint(c.endpoint)
			if c.wantOK && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !c.wantOK && err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestValidateEndpointAllowsLoopbackWhenPermitted(t *testing.T) {
	permissive := newEndpointGuard(nil, true, stubResolver{}, recordDial(&[]string{}))
	for _, endpoint := range []string{"http://127.0.0.1:9999/x", "http://localhost:9999/x", "http://[::1]:9999/x"} {
		if err := permissive.ValidateEndpoint(endpoint); err != nil {
			t.Fatalf("loopback %q must be allowed when permitted, got %v", endpoint, err)
		}
	}
	strict := newEndpointGuard(nil, false, stubResolver{}, recordDial(&[]string{}))
	for _, endpoint := range []string{"http://127.0.0.1:9999/x", "https://127.0.0.1:9999/x"} {
		if err := strict.ValidateEndpoint(endpoint); err == nil {
			t.Fatalf("loopback %q must be rejected without permission", endpoint)
		}
	}
}

func TestBlockedIPRejectsUnsafeRanges(t *testing.T) {
	blocked := []string{
		// IPv4 loopback, private, link-local (incl. cloud metadata), multicast,
		// broadcast, documentation, and unspecified.
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "172.31.255.254", "192.168.1.1",
		"169.254.0.1", "169.254.169.254", "224.0.0.1", "255.255.255.255",
		"0.1.2.3", "100.64.0.1", "100.127.255.254", "192.0.0.1",
		"192.0.2.1", "198.18.0.1", "198.19.255.254", "198.51.100.1",
		"203.0.113.1", "240.0.0.1", "255.255.255.255", "0.0.0.0",
		// IPv6 unspecified, loopback, unique-local, link-local, multicast,
		// documentation, and IPv4-mapped forms.
		"::", "::1", "fc00::1", "fd00::1", "fe80::1", "ff02::1", "2001:db8::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1",
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("bad test IP %q", raw)
		}
		if !ipBlocked(ip, false) {
			t.Errorf("%s must be blocked", raw)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("bad test IP %q", raw)
		}
		if ipBlocked(ip, false) {
			t.Errorf("%s must be allowed", raw)
		}
	}
	// Loopback becomes dialable only when explicitly permitted.
	if ipBlocked(net.ParseIP("127.0.0.1"), true) {
		t.Error("loopback must be allowed when permitted")
	}
}

func TestDialGuardRejectsPrivateDestinations(t *testing.T) {
	var dialed []string
	guard := newEndpointGuard([]string{"fcm.googleapis.com"}, false, stubResolver{"fcm.googleapis.com": {net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")}}, recordDial(&dialed))
	if _, err := guard.dialContext(context.Background(), "tcp", "fcm.googleapis.com:443"); err == nil {
		t.Fatal("a hostname resolving to private addresses must be rejected")
	}
	if len(dialed) != 0 {
		t.Fatalf("no dial may occur for a blocked destination, dialed %v", dialed)
	}
}

func TestDialGuardRejectsObfuscatedAndMetadataDestinations(t *testing.T) {
	literals := []string{"169.254.169.254", "0.0.0.0", "::1", "fc00::1", "::ffff:127.0.0.1", "192.168.0.1"}
	for _, host := range literals {
		guard := newEndpointGuard(nil, false, stubResolver{}, recordDial(&[]string{}))
		if _, err := guard.dialContext(context.Background(), "tcp", net.JoinHostPort(host, "443")); err == nil {
			t.Errorf("IP literal %s must be rejected", host)
		}
	}
	// An obfuscated numeric host whose resolution yields a blocked address is
	// rejected before any dial.
	var dialed []string
	guard := newEndpointGuard(nil, false, stubResolver{"2130706433": {net.ParseIP("127.0.0.1")}}, recordDial(&dialed))
	if _, err := guard.dialContext(context.Background(), "tcp", "2130706433:443"); err == nil {
		t.Fatal("obfuscated numeric host resolving to loopback must be rejected")
	}
	if len(dialed) != 0 {
		t.Fatalf("no dial may occur, dialed %v", dialed)
	}
}

func TestDialGuardPinsAllowedAddress(t *testing.T) {
	var dialed []string
	// The first candidate is blocked, so the dial must pin the second.
	guard := newEndpointGuard([]string{"fcm.googleapis.com"}, false, stubResolver{"fcm.googleapis.com": {net.ParseIP("10.0.0.1"), net.ParseIP("8.8.8.8")}}, recordDial(&dialed))
	if _, err := guard.dialContext(context.Background(), "tcp", "fcm.googleapis.com:443"); err == nil || !strings.Contains(err.Error(), "dial stub called") {
		t.Fatalf("expected the pinned dial to be attempted, got %v", err)
	}
	if len(dialed) != 1 || dialed[0] != "8.8.8.8:443" {
		t.Fatalf("expected a single pinned dial to 8.8.8.8:443, got %v", dialed)
	}
}

func TestDialGuardReResolvesPerDialAgainstRebinding(t *testing.T) {
	resolver := &flippingResolver{answers: [][]net.IP{{net.ParseIP("8.8.8.8")}, {net.ParseIP("127.0.0.1")}}}
	guard := newEndpointGuard([]string{"fcm.googleapis.com"}, false, resolver, recordDial(&[]string{}))
	if _, err := guard.dialContext(context.Background(), "tcp", "fcm.googleapis.com:443"); err == nil || !strings.Contains(err.Error(), "dial stub called") {
		t.Fatalf("first dial must reach the stub, got %v", err)
	}
	// The second dial re-resolves and must now be blocked.
	if _, err := guard.dialContext(context.Background(), "tcp", "fcm.googleapis.com:443"); err == nil || strings.Contains(err.Error(), "dial stub called") {
		t.Fatalf("second dial must be blocked after the answer flipped to a private address, got %v", err)
	}
}

func TestDialGuardRechecksHostAllowlist(t *testing.T) {
	var dialed []string
	guard := newEndpointGuard(
		[]string{"fcm.googleapis.com"},
		false,
		stubResolver{"evil.example": {net.ParseIP("8.8.8.8")}},
		recordDial(&dialed),
	)
	if _, err := guard.dialContext(context.Background(), "tcp", "evil.example:443"); err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("dial-time allowlist must reject an untrusted public host, got %v", err)
	}
	if len(dialed) != 0 {
		t.Fatalf("untrusted host reached dialer: %v", dialed)
	}
}

func TestRedirectsAreProhibited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://fcm.googleapis.com/fcm/send", http.StatusFound)
	}))
	t.Cleanup(server.Close)

	guard := NewEndpointGuard(nil, true)
	// The guarded client dials the loopback test server directly (IP literal)
	// and must fail the delivery when the server answers with a redirect.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/push", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp, doErr := guard.Client().Do(req); doErr == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("a redirected push endpoint must fail the delivery")
	}
	// CheckRedirect rejects every 3xx response outright.
	if err := guard.Client().CheckRedirect(req, []*http.Request{req}); err == nil {
		t.Fatal("CheckRedirect must prohibit redirects")
	}
}

func TestSenderRejectsEndpointOutsideAllowlist(t *testing.T) {
	keys, _ := GenerateKeyPair()
	guard := newEndpointGuard([]string{"fcm.googleapis.com"}, false, stubResolver{}, recordDial(&[]string{}))
	sender := NewSender(keys, "mailto:ops@example.com", guard, nil)
	sub := mustReceiverSubscription(t, "https://evil.example/push")
	if err := sender.Send(context.Background(), sub, []byte(`{}`)); !errors.Is(err, ErrSubscriptionGone) {
		t.Fatalf("expected ErrSubscriptionGone for a disallowed endpoint, got %v", err)
	}
}

func TestSenderDeliversToAllowlistedEndpoint(t *testing.T) {
	keys, _ := GenerateKeyPair()
	guard := newEndpointGuard([]string{"fcm.googleapis.com"}, false, stubResolver{}, recordDial(&[]string{}))
	called := false
	sender := NewSender(keys, "mailto:ops@example.com", guard, cannedDoer{status: http.StatusOK, called: &called})
	sub := mustReceiverSubscription(t, "https://fcm.googleapis.com/fcm/send/abc")
	if err := sender.Send(context.Background(), sub, []byte(`{}`)); err != nil {
		t.Fatalf("an allowlisted endpoint must deliver, got %v", err)
	}
	if !called {
		t.Fatal("expected the delivery client to be called")
	}
}

func TestSubscribeRejectsDisallowedHost(t *testing.T) {
	keys, _ := GenerateKeyPair()
	cfg := &config.Config{Environment: config.EnvTest, PushEndpointAllowlist: []string{"fcm.googleapis.com"}}
	svc := NewService(Deps{Store: &fakeStore{}, Keys: keys, Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	h := NewHTTP(svc)

	body := `{"endpoint":"https://evil.example/sub","keys":{"p256dh":"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`
	rec := httptest.NewRecorder()
	h.Subscribe(rec, userRequest(http.MethodPost, "/", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("subscribe status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}
