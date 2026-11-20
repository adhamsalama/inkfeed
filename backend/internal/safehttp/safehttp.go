// Package safehttp provides HTTP clients for fetching user-supplied URLs
// without exposing internal network services (SSRF protection).
//
// Destinations are checked at dial time against the resolved IP address, so
// redirects and DNS rebinding cannot be used to reach loopback, private,
// link-local (e.g. cloud metadata at 169.254.169.254) or other non-public
// addresses.
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a request targets a non-public address.
var ErrBlockedAddress = errors.New("destination address is not allowed")

var allowPrivate atomic.Bool

// SetAllowPrivateNetworks disables the guard. Intended for tests and local
// development only.
func SetAllowPrivateNetworks(allow bool) { allowPrivate.Store(allow) }

// Extra non-public ranges not covered by netip.Addr helper methods.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // "this" network
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64, can map to internal IPv4
}

// IsPublic reports whether ip is a publicly routable unicast address.
func IsPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func control(_, address string, _ syscall.RawConn) error {
	if allowPrivate.Load() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if !IsPublic(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
	}
	return nil
}

// NewTransport returns a new transport (based on http.DefaultTransport) that
// refuses to connect to non-public addresses. Use it when the transport needs
// further customisation; otherwise prefer NewClient.
func NewTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   control,
	}
	t.DialContext = d.DialContext
	return t
}

// Shared so connections are pooled across clients.
var sharedTransport = NewTransport()

// NewClient returns an HTTP client with the given timeout that refuses to
// connect to non-public addresses.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: sharedTransport}
}
