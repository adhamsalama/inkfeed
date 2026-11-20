package safehttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestIsPublic(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":            true,
		"1.1.1.1":            true,
		"2606:4700::1111":    true,
		"127.0.0.1":          false,
		"10.1.2.3":           false,
		"172.16.0.1":         false,
		"192.168.1.1":        false,
		"169.254.169.254":    false,
		"100.64.0.1":         false,
		"0.0.0.0":            false,
		"255.255.255.255":    false,
		"::1":                false,
		"fe80::1":            false,
		"fd00::1":            false,
		"::ffff:127.0.0.1":   false,
		"::ffff:8.8.8.8":     true,
		"64:ff9b::7f00:0001": false,
	}
	for s, want := range cases {
		if got := IsPublic(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsPublic(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := NewClient(5 * time.Second).Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("expected ErrBlockedAddress, got %v", err)
	}

	SetAllowPrivateNetworks(true)
	defer SetAllowPrivateNetworks(false)
	resp, err = NewClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("expected success with private networks allowed, got %v", err)
	}
	_ = resp.Body.Close()
}

func TestClientBlocksRedirectToLoopback(t *testing.T) {
	// Allowed origin that redirects to a blocked one: the redirect hop must
	// still be checked at dial time.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	SetAllowPrivateNetworks(true)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetAllowPrivateNetworks(false) // guard back on before the redirect hop
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	defer SetAllowPrivateNetworks(false)

	client := &http.Client{Timeout: 5 * time.Second, Transport: NewTransport()}
	resp, err := client.Get(redirector.URL)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("expected ErrBlockedAddress on redirect, got %v", err)
	}
}
