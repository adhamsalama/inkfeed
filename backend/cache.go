package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type cacheEntry struct {
	body        []byte
	contentType string
	expiresAt   time.Time
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

func (a *App) startCacheCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			a.cache.mu.Lock()
			for k, e := range a.cache.entries {
				if now.After(e.expiresAt) {
					delete(a.cache.entries, k)
				}
			}
			a.cache.mu.Unlock()
		}
	}()
}

// cached wraps a handler with a 5-minute in-memory cache keyed by the full request URL.
func (a *App) cached(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.String()

		a.cache.mu.Lock()
		entry, ok := a.cache.entries[key]
		if ok && time.Now().Before(entry.expiresAt) {
			fmt.Println("cache hit (in-memory) for key:", key)
			a.cache.mu.Unlock()
			w.Header().Set("Content-Type", entry.contentType)
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.Write(entry.body)
			return
		}
		a.cache.mu.Unlock()

		rec := &responseRecorder{header: make(http.Header)}
		next.ServeHTTP(rec, r)

		if rec.status == 0 || rec.status == http.StatusOK {
			a.cache.mu.Lock()
			a.cache.entries[key] = cacheEntry{
				body:        rec.body.Bytes(),
				contentType: rec.header.Get("Content-Type"),
				expiresAt:   time.Now().Add(5 * time.Minute),
			}
			a.cache.mu.Unlock()
		}

		for k, vals := range rec.header {
			for _, v := range vals {
				w.Header().Set(k, v)
			}
		}
		if rec.status != 0 {
			w.WriteHeader(rec.status)
		}
		w.Write(rec.body.Bytes())
	}
}

type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *responseRecorder) Header() http.Header         { return r.header }
func (r *responseRecorder) WriteHeader(status int)      { r.status = status }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
