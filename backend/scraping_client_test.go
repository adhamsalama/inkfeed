package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func mustParseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func TestNewScrappingClient(t *testing.T) {
	setProxyURL(t, "http://proxy.example")
	c := newScrappingClient(ScrappingClientConfig{Timeout: time.Second, WithProxy: true, UseProxyFirst: true})
	if c.ProxyURL == nil || *c.ProxyURL != "http://proxy.example" {
		t.Errorf("proxy url = %v", c.ProxyURL)
	}
	if c.UserAgent == nil || *c.UserAgent != userAgent {
		t.Errorf("user agent = %v", c.UserAgent)
	}

	// Without proxy
	c2 := newScrappingClient(ScrappingClientConfig{Timeout: time.Second, WithProxy: false})
	if c2.ProxyURL != nil {
		t.Errorf("expected nil proxy, got %v", c2.ProxyURL)
	}
}

func TestIsSuccessfulResponse(t *testing.T) {
	c := &ScrappingYoungLad{}
	if c.isSuccessfulResponse(nil, nil) {
		t.Error("nil resp not successful")
	}
	if c.isSuccessfulResponse(&http.Response{StatusCode: 200}, nil) != true {
		t.Error("200 should be successful")
	}
	if c.isSuccessfulResponse(&http.Response{StatusCode: 404}, nil) {
		t.Error("404 not successful")
	}
	if c.isSuccessfulResponse(&http.Response{StatusCode: 200}, http.ErrHandlerTimeout) {
		t.Error("error means not successful")
	}
}

func TestGenerateDirectAndProxyError(t *testing.T) {
	c := &ScrappingYoungLad{}
	err := c.generateDirectAndProxyError(http.ErrHandlerTimeout, http.ErrHandlerTimeout)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoDirectOnly(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	c := newScrappingClient(ScrappingClientConfig{Timeout: 5 * time.Second, WithProxy: false})
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestDoProxyFirstSuccess(t *testing.T) {
	proxy := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") == "" {
			t.Error("proxy did not receive url param")
		}
		w.Write([]byte("via proxy"))
	})
	setProxyURL(t, proxy.URL)
	c := newScrappingClient(ScrappingClientConfig{Timeout: 5 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, _ := http.NewRequest("GET", "https://target.example/page", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestDoProxyFirstFallbackToDirect(t *testing.T) {
	// Proxy fails (5xx); direct target succeeds.
	proxy := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	setProxyURL(t, proxy.URL)
	target := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("direct ok"))
	})
	c := newScrappingClient(ScrappingClientConfig{Timeout: 5 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, _ := http.NewRequest("GET", target.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestDoDirectFirstFallbackToProxy(t *testing.T) {
	proxy := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("proxy ok"))
	})
	setProxyURL(t, proxy.URL)
	c := newScrappingClient(ScrappingClientConfig{Timeout: 5 * time.Second, WithProxy: true, UseProxyFirst: false})
	req, _ := http.NewRequest("GET", "http://127.0.0.1:0/unreachable", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestDoBothFail(t *testing.T) {
	setProxyURL(t, "http://127.0.0.1:0/deadproxy")
	c := newScrappingClient(ScrappingClientConfig{Timeout: 2 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, _ := http.NewRequest("GET", "http://127.0.0.1:0/deadtarget", nil)
	if _, err := c.Do(req); err == nil {
		t.Error("expected error when both fail")
	}
}

var _ = httptest.NewServer

func TestDoProxyRequestBuildError(t *testing.T) {
	setProxyURL(t, "http://proxy.example")
	c := newScrappingClient(ScrappingClientConfig{Timeout: time.Second, WithProxy: true, UseProxyFirst: true})
	// An invalid method makes http.NewRequest inside doProxyRequest fail, and the
	// direct request also fails, so Do returns the combined error.
	req := &http.Request{Method: "BAD METHOD", URL: mustParseURL("http://127.0.0.1:0/x"), Header: http.Header{}}
	if _, err := c.Do(req); err == nil {
		t.Error("expected error for invalid method")
	}
}
