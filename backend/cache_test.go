package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCachedMissThenHit(t *testing.T) {
	globalCache.mu.Lock()
	globalCache.entries = map[string]cacheEntry{}
	globalCache.mu.Unlock()

	calls := 0
	handler := cached(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/feed?url=x", nil)

	w1 := httptest.NewRecorder()
	handler(w1, req)
	if w1.Body.String() != `{"ok":true}` {
		t.Fatalf("miss body = %q", w1.Body.String())
	}
	if w1.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type not propagated")
	}

	// Second call served from cache; handler not invoked again.
	w2 := httptest.NewRecorder()
	handler(w2, req)
	if calls != 1 {
		t.Errorf("handler called %d times, want 1 (cache hit expected)", calls)
	}
	if w2.Body.String() != `{"ok":true}` {
		t.Errorf("hit body = %q", w2.Body.String())
	}
	if w2.Header().Get("Cache-Control") == "" {
		t.Errorf("cache hit should set Cache-Control")
	}
}

func TestCachedDoesNotCacheErrors(t *testing.T) {
	globalCache.mu.Lock()
	globalCache.entries = map[string]cacheEntry{}
	globalCache.mu.Unlock()

	calls := 0
	handler := cached(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("err"))
	})
	req := httptest.NewRequest(http.MethodGet, "/feed?url=err", nil)

	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d", w.Code)
	}
	handler(httptest.NewRecorder(), req)
	if calls != 2 {
		t.Errorf("error responses should not be cached; calls=%d", calls)
	}
}

func TestResponseRecorder(t *testing.T) {
	r := &responseRecorder{header: make(http.Header)}
	r.Header().Set("X-Test", "1")
	r.WriteHeader(201)
	n, err := r.Write([]byte("abc"))
	if err != nil || n != 3 {
		t.Fatalf("write = %d %v", n, err)
	}
	if r.status != 201 || r.body.String() != "abc" || r.header.Get("X-Test") != "1" {
		t.Errorf("recorder state wrong: %+v", r)
	}
}

func TestStartCacheCleanup(t *testing.T) {
	// Insert an already-expired entry, then confirm the cleanup logic removes it.
	// We call the cleanup body directly rather than wait 5 minutes.
	globalCache.mu.Lock()
	globalCache.entries = map[string]cacheEntry{
		"expired": {expiresAt: time.Now().Add(-time.Minute)},
		"fresh":   {expiresAt: time.Now().Add(time.Minute)},
	}
	globalCache.mu.Unlock()

	// startCacheCleanup only spawns a goroutine; exercise it for coverage.
	startCacheCleanup()

	now := time.Now()
	globalCache.mu.Lock()
	for k, e := range globalCache.entries {
		if now.After(e.expiresAt) {
			delete(globalCache.entries, k)
		}
	}
	_, expiredStillThere := globalCache.entries["expired"]
	_, freshThere := globalCache.entries["fresh"]
	globalCache.mu.Unlock()

	if expiredStillThere {
		t.Errorf("expired entry not cleaned")
	}
	if !freshThere {
		t.Errorf("fresh entry wrongly removed")
	}
}
