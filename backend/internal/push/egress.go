package push

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IPResolver resolves hostnames to IP addresses. *net.Resolver satisfies it;
// tests inject a stub so the dial guard can be exercised without real DNS.
type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// EndpointGuard keeps Web Push delivery pinned to allowlisted provider domains
// and prevents SSRF. It performs two complementary checks:
//
//   - Endpoint validation (before storing a subscription or sending a message):
//     the endpoint host must match an allowlisted provider domain exactly or as
//     a subdomain, and HTTPS is required except for loopback destinations when
//     loopback is permitted (the isolated test stack).
//   - Dial-time protection: the hardened transport resolves the destination
//     itself, refuses to dial loopback, private, link-local (including cloud
//     metadata), multicast, documentation, or unspecified addresses in IPv4
//     and IPv6, pins the first allowed address for each dial, and never
//     follows redirects. Resolution happens inside every dial so a DNS answer
//     flipped between sends cannot smuggle a private destination through.
type EndpointGuard struct {
	allowlist     []string
	allowLoopback bool
	resolver      IPResolver
	dial          dialFunc
	client        *http.Client
}

// NewEndpointGuard builds a guard with the system resolver and the production
// dialer. allowLoopback permits loopback destinations; production passes false
// so no request can target the host itself.
func NewEndpointGuard(allowlist []string, allowLoopback bool) *EndpointGuard {
	return newEndpointGuard(allowlist, allowLoopback, net.DefaultResolver, defaultDial)
}

func newEndpointGuard(allowlist []string, allowLoopback bool, resolver IPResolver, dial dialFunc) *EndpointGuard {
	normalized := make([]string, 0, len(allowlist))
	for _, domain := range allowlist {
		if domain = strings.ToLower(strings.TrimSpace(domain)); domain != "" {
			normalized = append(normalized, domain)
		}
	}
	g := &EndpointGuard{allowlist: normalized, allowLoopback: allowLoopback, resolver: resolver, dial: dial}
	transport := &http.Transport{
		// Never route push traffic through a proxy: the dial guard must see the
		// real destination instead of trusting whatever the proxy resolves.
		Proxy:                 nil, // never route push traffic through a proxy: the dial guard must see the real destination
		DialContext:           g.dialContext,
		DisableKeepAlives:     true,                                                   // a fresh dial per send defeats DNS rebinding
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{}, // HTTP/1.1 only: no pooled connections to reuse
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	g.client = &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Any 3xx aborts the delivery: a push endpoint that redirects is
			// either misconfigured or an SSRF lure, and following it would
			// re-dial an unvalidated destination.
			return errors.New("push delivery: redirects prohibited")
		},
	}
	return g
}

func defaultDial(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
}

// Client returns the hardened HTTP client. Delivery MUST use this client so
// every dial goes through the address guard and no redirect is followed.
func (g *EndpointGuard) Client() *http.Client { return g.client }

// HostAllowed reports whether host may be a push destination: it matches the
// allowlist exactly or as a subdomain, or (when loopback is permitted) is a
// loopback address.
func (g *EndpointGuard) HostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if isLoopbackHost(host) {
		return g.allowLoopback
	}
	for _, domain := range g.allowlist {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// ValidateEndpoint checks an endpoint URL before it is stored or sent: HTTPS is
// required except for loopback destinations when loopback is permitted, and the
// host must be allowlisted.
func (g *EndpointGuard) ValidateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return errors.New("endpoint must be an absolute URL")
	}
	host := parsed.Hostname()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && g.allowLoopback && isLoopbackHost(host)) {
		return errors.New("endpoint must use HTTPS")
	}
	if !g.HostAllowed(host) {
		return errors.New("endpoint host is not an allowlisted push provider")
	}
	if port := parsed.Port(); port != "" && port != "443" && !(g.allowLoopback && isLoopbackHost(host)) {
		return errors.New("endpoint must use the HTTPS default port")
	}
	return nil
}

// dialContext resolves the destination and connects only to an allowed
// address. Resolution happens inside the dial so each new connection sees the
// current DNS answer; pinning per dial defeats rebinding.
func (g *EndpointGuard) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("push dial: invalid address %q: %w", addr, err)
	}
	if !g.HostAllowed(host) {
		return nil, fmt.Errorf("push dial: host %q is not allowlisted", host)
	}
	if port != "443" && !(g.allowLoopback && isLoopbackHost(host)) {
		return nil, fmt.Errorf("push dial: port %q is not allowed", port)
	}
	if ip := net.ParseIP(host); ip != nil {
		if ipBlocked(ip, g.allowLoopback) {
			return nil, fmt.Errorf("push dial: destination %s is blocked", ip)
		}
		return g.dial(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	addrs, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("push dial: resolve %s: %w", host, err)
	}
	var lastDialErr error
	for _, resolved := range addrs {
		ip := resolved.IP
		if ip == nil || ipBlocked(ip, g.allowLoopback) {
			continue
		}
		conn, dialErr := g.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastDialErr = dialErr
		if ctx.Err() != nil {
			return nil, fmt.Errorf("push dial: connect %s: %w", host, ctx.Err())
		}
	}
	if lastDialErr != nil {
		return nil, fmt.Errorf("push dial: all allowed addresses for %s failed: %w", host, lastDialErr)
	}
	return nil, fmt.Errorf("push dial: no allowed address for %s", host)
}

// ipBlocked reports whether ip may never be dialed: loopback (unless loopback
// is explicitly permitted), private, link-local (which includes cloud metadata
// 169.254.169.254), multicast, documentation, unspecified, or IPv4-mapped
// loopback addresses. The IPv4-mapped forms are normalized first so Go's
// per-family checks apply to them.
func ipBlocked(ip net.IP, allowLoopback bool) bool {
	if ip == nil || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4.IsLoopback() {
			return !allowLoopback
		}
		return ip4.IsPrivate() || ip4.IsLinkLocalUnicast() || ip4.IsMulticast() || isIPv4SpecialUse(ip4)
	}
	if ip.IsLoopback() {
		return !allowLoopback
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || isIPv6Documentation(ip)
}

// isIPv4SpecialUse rejects non-public IANA special-purpose ranges which Go's
// IsPrivate/IsGlobalUnicast predicates intentionally do not classify as
// private. In particular, carrier-grade NAT and benchmarking space can route
// to infrastructure that must never be reachable through a stored endpoint.
func isIPv4SpecialUse(ip net.IP) bool {
	return ip[0] == 0 ||
		(ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127) ||
		(ip[0] == 192 && ip[1] == 0 && ip[2] == 0) ||
		(ip[0] == 198 && (ip[1] == 18 || ip[1] == 19)) ||
		ip[0] >= 240 || isIPv4Documentation(ip)
}

// isIPv4Documentation reports whether ip is inside RFC 5737 documentation space.
func isIPv4Documentation(ip net.IP) bool {
	return (ip[0] == 192 && ip[1] == 0 && ip[2] == 2) ||
		(ip[0] == 198 && ip[1] == 51 && ip[2] == 100) ||
		(ip[0] == 203 && ip[1] == 0 && ip[2] == 113)
}

// isIPv4Broadcast reports whether ip is the IPv4 limited broadcast address.
// isIPv6Documentation reports whether ip is inside the RFC 3849 2001:db8::/32
// documentation prefix.
func isIPv6Documentation(ip net.IP) bool {
	return len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8
}
