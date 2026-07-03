package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsAllowedOrigin(t *testing.T) {
	if !isAllowedOrigin("https://reader.inkfeed.xyz") {
		t.Error("prod origin should be allowed")
	}
	if isAllowedOrigin("https://evil.com") {
		t.Error("unknown origin should be rejected")
	}
	if isAllowedOrigin("") {
		t.Error("empty origin should be rejected")
	}
}

func TestCorsMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := corsMiddleware(next)

	// Disallowed origin
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("disallowed origin = %d", w.Code)
	}

	// Allowed origin, normal request
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://reader.inkfeed.xyz")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTeapot {
		t.Errorf("allowed origin = %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://reader.inkfeed.xyz" {
		t.Errorf("ACAO header missing")
	}

	// OPTIONS preflight
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://reader.inkfeed.xyz")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight = %d", w.Code)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	called := false
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/some/path", nil))
	if !called || w.Code != http.StatusCreated {
		t.Errorf("logging middleware: called=%v code=%d", called, w.Code)
	}
}

func TestStatusRecorder(t *testing.T) {
	inner := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: inner, status: 200}
	sr.WriteHeader(404)
	if sr.status != 404 || inner.Code != 404 {
		t.Errorf("statusRecorder: sr=%d inner=%d", sr.status, inner.Code)
	}
}

func TestApplyEnvConfig(t *testing.T) {
	origOrigins := allowedOrigins
	origProxy := feedProxyURL
	defer func() {
		allowedOrigins = origOrigins
		feedProxyURL = origProxy
	}()

	os.Setenv("ALLOWED_ORIGINS", "https://a.com,https://b.com")
	os.Setenv("ENV", "local")
	os.Setenv("FEED_PROXY_URL", "https://proxy.test")
	defer func() {
		os.Unsetenv("ALLOWED_ORIGINS")
		os.Unsetenv("ENV")
		os.Unsetenv("FEED_PROXY_URL")
	}()

	applyEnvConfig()
	if !isAllowedOrigin("https://a.com") || !isAllowedOrigin("http://localhost:8000") {
		t.Errorf("origins not applied: %v", allowedOrigins)
	}
	if feedProxyURL != "https://proxy.test" {
		t.Errorf("proxy = %q", feedProxyURL)
	}
}

func TestSetupDB(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := setupDB(filepath.Join(dir, "setup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	// Idempotent: running again on the same file must not error.
	sqlDB2, err := setupDB(filepath.Join(dir, "setup.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB2.Close()

	// Invalid path errors.
	if _, err := setupDB("/nonexistent-dir/does/not/exist.db"); err == nil {
		t.Error("expected error for bad path")
	}
}

func TestNewServeMux(t *testing.T) {
	mux := newServeMux()
	// Unauthenticated request to a protected route with an allowed origin should
	// reach authMiddleware and be rejected as unauthorized (proving routing +
	// middleware are wired).
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	req.Header.Set("Origin", "https://reader.inkfeed.xyz")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("protected route without auth = %d, want 401", w.Code)
	}

	// A disallowed origin is blocked by CORS.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/feed", nil)
	req.Header.Set("Origin", "https://evil.com")
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("disallowed origin = %d, want 403", w.Code)
	}
}

func TestStartBackgroundJobs(t *testing.T) {
	resetDB(t)
	// Just ensure it launches without panicking. The goroutines operate on the
	// empty test DB (no saved feeds) so they are effectively no-ops.
	startBackgroundJobs()
}

func TestJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	jsonError(w, "boom", http.StatusBadRequest)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type = %q", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "{\"error\":\"boom\"}\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}
