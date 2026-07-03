package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/adhamsalama/inkfeed-backend/db"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

var queries *db.Queries

var allowedOrigins = []string{"https://reader.inkfeed.xyz", "http://reader.inkfeed.xyz", "http://localhost:9999"}

var feedProxyURL = "https://throbbing-morning-e187.adhamsalama.workers.dev"

type contextKey string

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

func isAllowedOrigin(origin string) bool {
	for _, o := range allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !isAllowedOrigin(origin) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := url.PathUnescape(r.URL.RequestURI())
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, path, rec.status, clientIP(r))
	})
}

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// applyEnvConfig reads process configuration from environment variables. It is
// separated from main so it can be exercised in tests.
func applyEnvConfig() {
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		allowedOrigins = strings.Split(v, ",")
	}
	if os.Getenv("ENV") == "local" {
		allowedOrigins = append(allowedOrigins, "http://localhost:8000")
	}
	if v := os.Getenv("FEED_PROXY_URL"); v != "" {
		feedProxyURL = v
	}
}

// setupDB opens the SQLite database at path, applies pragmas, and runs the
// idempotent schema migrations. It returns the connection ready for db.New.
func setupDB(path string) (*sql.DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER  PRIMARY KEY AUTOINCREMENT,
			email         TEXT     NOT NULL UNIQUE,
			password_hash TEXT     NOT NULL,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT     PRIMARY KEY,
			user_id    INTEGER  NOT NULL REFERENCES users(id),
			expires_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS user_preferences (
			user_id           INTEGER PRIMARY KEY REFERENCES users(id),
			font_size         REAL,
			letter_spacing    REAL,
			line_height       REAL,
			cors_proxy_url    TEXT,
			epub_embed_images INTEGER,
			mobi_embed_images INTEGER,
			email_to          TEXT,
			updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS user_saved_feeds (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id  INTEGER NOT NULL REFERENCES users(id),
			url      TEXT    NOT NULL,
			title    TEXT    NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS user_feed_groups (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id  INTEGER NOT NULL REFERENCES users(id),
			name     TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS user_feed_group_items (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL REFERENCES user_feed_groups(id),
			url      TEXT NOT NULL,
			title    TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS user_favorites (
			id         INTEGER  PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER  NOT NULL REFERENCES users(id),
			url        TEXT     NOT NULL,
			title      TEXT     NOT NULL DEFAULT '',
			feed_title TEXT     NOT NULL DEFAULT '',
			pub_date   TEXT     NOT NULL DEFAULT '',
			saved_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	); err != nil {
		return nil, err
	}
	// SQLite doesn't support IF NOT EXISTS on ALTER TABLE; errors are intentionally swallowed.
	migrate := func(query string) { _, _ = sqlDB.Exec(query) }

	migrate(`ALTER TABLE user_preferences ADD COLUMN email_to TEXT`)
	migrate(`ALTER TABLE user_preferences ADD COLUMN mobi_embed_images INTEGER`)
	migrate(`DROP TABLE IF EXISTS persistent_cache`)
	migrate(`ALTER TABLE user_favorites ADD COLUMN comments_url TEXT NOT NULL DEFAULT ''`)
	migrate(`CREATE TABLE IF NOT EXISTS article_archive (key TEXT PRIMARY KEY, title TEXT NOT NULL DEFAULT '', author TEXT NOT NULL DEFAULT '', site_name TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', html_content TEXT NOT NULL DEFAULT '', text_content TEXT NOT NULL DEFAULT '', archived_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	migrate(`ALTER TABLE article_archive DROP COLUMN body`)
	migrate(`ALTER TABLE feed_items ADD COLUMN archive_failed INTEGER NOT NULL DEFAULT 0`)
	migrate(`ALTER TABLE feed_items ADD COLUMN comments_url TEXT`)
	migrate(`ALTER TABLE user_saved_feeds ADD COLUMN archive_enabled INTEGER NOT NULL DEFAULT 0`)
	migrate(`ALTER TABLE user_preferences ADD COLUMN font_family TEXT`)
	migrate(`ALTER TABLE user_preferences ADD COLUMN bold_text INTEGER`)
	migrate(`ALTER TABLE user_preferences ADD COLUMN dark_mode INTEGER`)
	migrate(`CREATE TABLE IF NOT EXISTS ip_rate_limits (ip TEXT NOT NULL, endpoint TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, window_start DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, blocked_until DATETIME, PRIMARY KEY (ip, endpoint))`)
	sqlDB.Exec(`CREATE TABLE IF NOT EXISTS feed_items (
		id             INTEGER  PRIMARY KEY AUTOINCREMENT,
		feed_url       TEXT     NOT NULL,
		item_url       TEXT     NOT NULL,
		title          TEXT     NOT NULL DEFAULT '',
		description    TEXT     NOT NULL DEFAULT '',
		pub_date       TEXT     NOT NULL DEFAULT '',
		scraped_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		archive_failed INTEGER  NOT NULL DEFAULT 0,
		UNIQUE(feed_url, item_url)
	)`)

	return sqlDB, nil
}

// startBackgroundJobs launches the periodic goroutines (feed scraping, content
// archiving, cache cleanup, and pruning).
func startBackgroundJobs() {
	startFeedScraper()
	startContentArchiver()
	startCacheCleanup()
	startArticleArchivePruner()
	startFeedItemsPruner()
}

// newServeMux builds the HTTP router with all routes and their middleware.
func newServeMux() *http.ServeMux {
	mux := http.NewServeMux()
	protected := func(h http.HandlerFunc) http.Handler {
		return corsMiddleware(authMiddleware(rateLimitMiddleware(http.HandlerFunc(h))))
	}

	mux.Handle("/signup", corsMiddleware(signupRateLimitMiddleware(http.HandlerFunc(signupHandler))))
	mux.Handle("/signin", corsMiddleware(signinRateLimitMiddleware(http.HandlerFunc(signinHandler))))
	mux.Handle("/signout", corsMiddleware(http.HandlerFunc(signoutHandler)))
	mux.Handle("/change-password", corsMiddleware(authMiddleware(signinRateLimitMiddleware(http.HandlerFunc(changePasswordHandler)))))
	mux.Handle("/preferences", protected(preferencesHandler))
	mux.Handle("/saved-feeds", protected(savedFeedsHandler))
	mux.Handle("/feed-groups", protected(feedGroupsHandler))
	mux.Handle("/favorites", protected(favoritesHandler))
	mux.Handle("/feed", protected(cached(feedHandler)))
	mux.Handle("/article", protected(cached(articleHandler)))
	mux.Handle("/text", protected(textHandler))
	mux.Handle("/comments", protected(cached(commentsHandler)))
	mux.Handle("/mobi", protected(mobiHandler))
	mux.Handle("/epub", protected(epubHandler))
	mux.Handle("/reddit-post", protected(redditPostHandler))
	mux.Handle("/decode-google-news", protected(decodeGoogleNewsHandler))
	mux.Handle("/email", corsMiddleware(authMiddleware(emailRateLimitMiddleware(http.HandlerFunc(emailHandler)))))
	mux.Handle("/feed-archive", protected(feedArchiveHandler))

	return mux
}

func main() {
	godotenv.Load()
	applyEnvConfig()

	port := flag.String("port", "8080", "port to listen on")
	flag.Parse()
	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}

	sqlDB, err := setupDB("inkfeed.db")
	if err != nil {
		log.Fatalf("failed to set up database: %v", err)
	}
	queries = db.New(sqlDB)

	startBackgroundJobs()

	addr := ":" + *port
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, loggingMiddleware(newServeMux())))
}
