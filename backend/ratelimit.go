package main

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/db"
)

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		return r.RemoteAddr
	}
	return ip
}

// dbRateLimit checks and updates a persistent rate limit for the given IP and endpoint.
// Returns true if the request is allowed, false if it should be blocked.
// limit is the max requests allowed in window. blockDuration is how long to block after exceeding.
func (a *App) dbRateLimit(ctx context.Context, mu *sync.Mutex, ip, endpoint string, limit int, window, blockDuration time.Duration) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()

	row, err := a.q.GetIPRateLimit(ctx, db.GetIPRateLimitParams{Ip: ip, Endpoint: endpoint})
	if err == sql.ErrNoRows {
		a.q.UpsertIPRateLimit(ctx, db.UpsertIPRateLimitParams{
			Ip:           ip,
			Endpoint:     endpoint,
			Count:        1,
			WindowStart:  now,
			BlockedUntil: sql.NullTime{},
		})
		return true
	}
	if err != nil {
		return true
	}

	if row.BlockedUntil.Valid && now.Before(row.BlockedUntil.Time) {
		return false
	}

	var count int64
	var windowStart time.Time
	if !row.BlockedUntil.Valid || now.After(row.BlockedUntil.Time) {
		if now.Sub(row.WindowStart) > window {
			count = 1
			windowStart = now
		} else {
			count = row.Count + 1
			windowStart = row.WindowStart
		}
	}

	if count > int64(limit) {
		a.q.UpsertIPRateLimit(ctx, db.UpsertIPRateLimitParams{
			Ip:           ip,
			Endpoint:     endpoint,
			Count:        count,
			WindowStart:  windowStart,
			BlockedUntil: sql.NullTime{Time: now.Add(blockDuration), Valid: true},
		})
		return false
	}

	a.q.UpsertIPRateLimit(ctx, db.UpsertIPRateLimitParams{
		Ip:           ip,
		Endpoint:     endpoint,
		Count:        count,
		WindowStart:  windowStart,
		BlockedUntil: sql.NullTime{},
	})
	return true
}

func (a *App) emailRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		a.emailRlMu.Lock()
		last, seen := a.emailRlHits[ip]
		if seen && time.Since(last) < time.Minute {
			a.emailRlMu.Unlock()
			jsonError(w, "email rate limit exceeded: 1 per minute", http.StatusTooManyRequests)
			return
		}
		a.emailRlHits[ip] = time.Now()
		a.emailRlMu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func (a *App) signupRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !a.dbRateLimit(r.Context(), &a.signupRlMu, ip, "signup", a.signupRlMax, a.authRlWindow, a.authRlBlock) {
			jsonError(w, "rate limit exceeded: too many signup attempts", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) signinRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !a.dbRateLimit(r.Context(), &a.signinRlMu, ip, "signin", a.signinRlMax, a.authRlWindow, a.authRlBlock) {
			jsonError(w, "rate limit exceeded: too many signin attempts", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		a.rlMu.Lock()
		now := time.Now()
		cutoff := now.Add(-a.rlWindow)
		hits := a.rlHits[ip]
		filtered := hits[:0]
		for _, t := range hits {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		a.rlHits[ip] = filtered
		if len(filtered) >= a.rlMax {
			a.rlMu.Unlock()
			jsonError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		a.rlHits[ip] = append(filtered, now)
		a.rlMu.Unlock()

		next.ServeHTTP(w, r)
	})
}
