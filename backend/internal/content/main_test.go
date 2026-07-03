package content

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

var (
	testDB *sql.DB
	svc    *Service
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "content-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000`); err != nil {
		panic(err)
	}
	schema, err := os.ReadFile("../../db/schema.sql")
	if err != nil {
		panic(err)
	}
	if _, err := sqlDB.Exec(string(schema)); err != nil {
		panic(err)
	}

	testDB = sqlDB
	svc = New(db.New(sqlDB))

	code := m.Run()
	sqlDB.Close()
	os.Exit(code)
}

func resetDB(t *testing.T) {
	t.Helper()
	for _, tbl := range []string{"users", "user_saved_feeds", "article_archive", "feed_items"} {
		if _, err := testDB.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("clear %s: %v", tbl, err)
		}
	}
}

func createTestUser(t *testing.T, email string) int64 {
	t.Helper()
	u, err := svc.q.CreateUser(context.Background(), db.CreateUserParams{Email: email, PasswordHash: "x"})
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	return u.ID
}

func feedArchiveParams(feedURL string) db.GetFeedArchiveItemsParams {
	return db.GetFeedArchiveItemsParams{FeedUrl: feedURL, Limit: 100, Offset: 0}
}

func savedFeedParams(userID int64, url string) db.InsertUserSavedFeedParams {
	return db.InsertUserSavedFeedParams{UserID: userID, Url: url, Title: url, Position: 0, ArchiveEnabled: 1}
}

func scrapeFeedInsert(t *testing.T, feedURL, itemURL string) {
	t.Helper()
	if _, err := svc.q.InsertFeedItem(context.Background(), db.InsertFeedItemParams{
		FeedUrl: feedURL, ItemUrl: itemURL, Title: "T", Description: "D", PubDate: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("scrapeFeedInsert: %v", err)
	}
}

// setProxyURL points the service's proxy at url for the test's duration.
func setProxyURL(t *testing.T, url string) {
	t.Helper()
	prev := svc.ProxyURL
	svc.ProxyURL = url
	t.Cleanup(func() { svc.ProxyURL = prev })
}

func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}
