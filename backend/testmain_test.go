package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/adhamsalama/inkfeed-backend/db"
	_ "modernc.org/sqlite"
)

// testDB is the shared *sql.DB and app is the shared App under test.
var testDB *sql.DB
var app *App

// TestMain sets up an on-disk SQLite database (so a single connection keeps the
// schema between calls) initialized from db/schema.sql, and wires it into the
// package-global queries variable that all handlers use.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "inkfeed-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	dbPath := filepath.Join(dir, "test.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000`); err != nil {
		panic(err)
	}

	schema, err := os.ReadFile("db/schema.sql")
	if err != nil {
		panic(err)
	}
	if _, err := sqlDB.Exec(string(schema)); err != nil {
		panic(err)
	}

	testDB = sqlDB
	app = newApp(db.New(sqlDB))

	code := m.Run()
	sqlDB.Close()
	os.Exit(code)
}

// resetDB truncates every table so each test starts from a clean slate.
func resetDB(t *testing.T) {
	t.Helper()
	tables := []string{
		"users", "sessions", "user_preferences", "user_saved_feeds",
		"user_feed_groups", "user_feed_group_items", "user_favorites",
		"article_archive", "ip_rate_limits", "feed_items",
	}
	for _, tbl := range tables {
		if _, err := testDB.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("failed to clear %s: %v", tbl, err)
		}
	}
}

// createTestUser inserts a user and returns its ID.
func createTestUser(t *testing.T, email string) int64 {
	t.Helper()
	u, err := app.q.CreateUser(context.Background(), db.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$fakehashfakehashfakehashfakehashfakehashfakehashfa",
	})
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	return u.ID
}

// feedArchiveParams builds default pagination params for the feed archive query.
func feedArchiveParams(feedURL string) db.GetFeedArchiveItemsParams {
	return db.GetFeedArchiveItemsParams{FeedUrl: feedURL, Limit: 100, Offset: 0}
}

// savedFeedParams builds an archive-enabled saved-feed insert for the user.
func savedFeedParams(userID int64, url string) db.InsertUserSavedFeedParams {
	return db.InsertUserSavedFeedParams{UserID: userID, Url: url, Title: url, Position: 0, ArchiveEnabled: 1}
}

// scrapeFeedInsert inserts a single feed_items row directly.
func scrapeFeedInsert(t *testing.T, feedURL, itemURL string) {
	t.Helper()
	if _, err := app.q.InsertFeedItem(context.Background(), db.InsertFeedItemParams{
		FeedUrl: feedURL, ItemUrl: itemURL, Title: "T", Description: "D", PubDate: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("scrapeFeedInsert: %v", err)
	}
}

// dbCreateUser builds CreateUserParams (small convenience for tests).
func dbCreateUser(email, hash string) db.CreateUserParams {
	return db.CreateUserParams{Email: email, PasswordHash: hash}
}

// userContext returns a context carrying the userID key that authMiddleware sets.
func userContext(userID int64) context.Context {
	return context.WithValue(context.Background(), contextKey("userID"), userID)
}

// setProxyURL points the package-global feedProxyURL at url for the duration of
// the test, restoring the previous value on cleanup. The scraping client
// (UseProxyFirst) routes through feedProxyURL, so a test httptest server URL
// exercises the network code paths without hitting the internet.
func setProxyURL(t *testing.T, url string) {
	t.Helper()
	prev := app.proxyURL
	app.proxyURL = url
	t.Cleanup(func() { app.proxyURL = prev })
}

// newTestServer is a thin wrapper around httptest.NewServer registered for
// cleanup.
func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}
