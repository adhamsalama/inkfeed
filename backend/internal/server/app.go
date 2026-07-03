package server

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/db"
)

// userAgent is sent on every outbound scraping/image request.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// defaultAllowedOrigins and defaultProxyURL are the baseline configuration,
// overridable via environment variables (see App.applyEnvConfig).
var defaultAllowedOrigins = []string{"https://reader.inkfeed.xyz", "http://reader.inkfeed.xyz", "http://localhost:9999"}

const defaultProxyURL = "https://throbbing-morning-e187.adhamsalama.workers.dev"

// App holds all runtime dependencies (database, cache, config, rate-limit
// state). Handlers and helpers are methods on *App so nothing relies on package
// globals — this makes the components independently testable and would let the
// package be split into sub-packages later.
type App struct {
	q     *db.Queries
	cache *responseCache

	allowedOrigins []string
	proxyURL       string

	// Rate limiting.
	rlMu         sync.Mutex
	rlHits       map[string][]time.Time
	rlMax        int
	rlWindow     time.Duration
	emailRlMu    sync.Mutex
	emailRlHits  map[string]time.Time
	signinRlMu   sync.Mutex
	signupRlMu   sync.Mutex
	signinRlMax  int
	signupRlMax  int
	authRlWindow time.Duration
	authRlBlock  time.Duration
}

// newApp builds an App with default configuration wired to the given queries,
// then applies rate-limit overrides from the environment.
func newApp(q *db.Queries) *App {
	a := &App{
		q:              q,
		cache:          &responseCache{entries: make(map[string]cacheEntry)},
		allowedOrigins: append([]string(nil), defaultAllowedOrigins...),
		proxyURL:       defaultProxyURL,
		rlHits:         make(map[string][]time.Time),
		emailRlHits:    make(map[string]time.Time),
		rlMax:          40,
		rlWindow:       time.Minute,
		signinRlMax:    10,
		signupRlMax:    1000,
		authRlWindow:   time.Hour,
		authRlBlock:    2 * time.Hour,
	}
	a.loadRateLimitEnv()
	return a
}

// loadRateLimitEnv reads the rate-limit tuning knobs from the environment.
func (a *App) loadRateLimitEnv() {
	if v := os.Getenv("RATE_LIMIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.rlMax = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.rlWindow = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("SIGNIN_RATE_LIMIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.signinRlMax = n
		}
	}
	if v := os.Getenv("SIGNUP_RATE_LIMIT_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.signupRlMax = n
		}
	}
	if v := os.Getenv("AUTH_RATE_LIMIT_WINDOW_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.authRlWindow = time.Duration(n) * time.Hour
		}
	}
	if v := os.Getenv("AUTH_RATE_LIMIT_BLOCK_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			a.authRlBlock = time.Duration(n) * time.Hour
		}
	}
}

// applyEnvConfig applies CORS origin and proxy overrides from the environment.
func (a *App) applyEnvConfig() {
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		a.allowedOrigins = strings.Split(v, ",")
	}
	if os.Getenv("ENV") == "local" {
		a.allowedOrigins = append(a.allowedOrigins, "http://localhost:8000")
	}
	if v := os.Getenv("FEED_PROXY_URL"); v != "" {
		a.proxyURL = v
	}
}
