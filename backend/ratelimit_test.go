package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"xff single", "9.9.9.9", "1.2.3.4:5678", "9.9.9.9"},
		{"xff list", "9.9.9.9, 8.8.8.8", "1.2.3.4:5678", "9.9.9.9"},
		{"remote addr", "", "1.2.3.4:5678", "1.2.3.4"},
		{"remote addr no port", "", "1.2.3.4", "1.2.3.4"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.remoteAddr
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(req); got != c.want {
			t.Errorf("%s: clientIP = %q, want %q", c.name, got, c.want)
		}
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRateLimitMiddleware(t *testing.T) {
	// Isolate global state for this test.
	app.rlMu.Lock()
	app.rlHits = map[string][]time.Time{}
	origMax := app.rlMax
	app.rlMax = 3
	app.rlMu.Unlock()
	defer func() {
		app.rlMu.Lock()
		app.rlMax = origMax
		app.rlHits = map[string][]time.Time{}
		app.rlMu.Unlock()
	}()

	h := app.rateLimitMiddleware(okHandler())
	do := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "5.5.5.5:1"
		h.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 3; i++ {
		if code := do(); code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i, code)
		}
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Errorf("over-limit request = %d, want 429", code)
	}
}

func TestEmailRateLimitMiddleware(t *testing.T) {
	app.emailRlMu.Lock()
	app.emailRlHits = map[string]time.Time{}
	app.emailRlMu.Unlock()

	h := app.emailRateLimitMiddleware(okHandler())
	do := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "6.6.6.6:1"
		h.ServeHTTP(w, req)
		return w.Code
	}
	if code := do(); code != http.StatusOK {
		t.Fatalf("first email = %d", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Errorf("second email within a minute = %d, want 429", code)
	}
}

func TestDBRateLimit(t *testing.T) {
	resetDB(t)
	var mu sync.Mutex
	ctx := context.Background()
	ip := "7.7.7.7"

	// First call: creates a row, allowed.
	if !app.dbRateLimit(ctx, &mu, ip, "signin", 3, time.Hour, time.Hour) {
		t.Fatal("first call should be allowed")
	}
	// Up to the limit: allowed.
	allowed := 1
	for i := 0; i < 2; i++ {
		if app.dbRateLimit(ctx, &mu, ip, "signin", 3, time.Hour, time.Hour) {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("expected 3 allowed, got %d", allowed)
	}
	// Next call exceeds -> blocked.
	if app.dbRateLimit(ctx, &mu, ip, "signin", 3, time.Hour, time.Hour) {
		t.Error("4th call should be blocked")
	}
	// Once blocked, still blocked.
	if app.dbRateLimit(ctx, &mu, ip, "signin", 3, time.Hour, time.Hour) {
		t.Error("call while blocked should be denied")
	}
}

func TestDBRateLimitWindowReset(t *testing.T) {
	resetDB(t)
	var mu sync.Mutex
	ctx := context.Background()
	// Very short window so it resets between calls.
	if !app.dbRateLimit(ctx, &mu, "8.8.8.8", "signup", 1, time.Nanosecond, time.Hour) {
		t.Fatal("first allowed")
	}
	time.Sleep(2 * time.Millisecond)
	// Window elapsed -> count resets to 1, allowed again.
	if !app.dbRateLimit(ctx, &mu, "8.8.8.8", "signup", 1, time.Nanosecond, time.Hour) {
		t.Error("should be allowed after window reset")
	}
}

func TestSignupSigninRateLimitMiddleware(t *testing.T) {
	resetDB(t)
	// signup middleware allows within default high limit.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signup", nil)
	req.RemoteAddr = "10.0.0.1:1"
	app.signupRateLimitMiddleware(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("signup rl = %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/signin", nil)
	req.RemoteAddr = "10.0.0.2:1"
	app.signinRateLimitMiddleware(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("signin rl = %d", w.Code)
	}
}

func TestSigninRateLimitMiddlewareBlocks(t *testing.T) {
	resetDB(t)
	origMax := app.signinRlMax
	app.signinRlMax = 1
	defer func() { app.signinRlMax = origMax }()

	h := app.signinRateLimitMiddleware(okHandler())
	do := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/signin", nil)
		req.RemoteAddr = "10.0.0.9:1"
		h.ServeHTTP(w, req)
		return w.Code
	}
	do()
	if code := do(); code != http.StatusTooManyRequests {
		t.Errorf("expected block, got %d", code)
	}
}
