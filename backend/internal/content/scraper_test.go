package content

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestFeedScraperConfigDefaults(t *testing.T) {
	for _, v := range []string{"FEED_SCRAPE_INTERVAL_HOURS", "FEED_ITEMS_MAX_AGE_HOURS", "FEED_ITEMS_PRUNE_INTERVAL_HOURS", "CONTENT_ARCHIVER_TIMEOUT_SECONDS"} {
		os.Unsetenv(v)
	}
	if feedScrapeInterval() != time.Hour {
		t.Errorf("scrape interval default = %v", feedScrapeInterval())
	}
	if feedItemsMaxAgeHours() != 14*24 {
		t.Errorf("max age default = %d", feedItemsMaxAgeHours())
	}
	if feedItemsPruneInterval() != time.Hour {
		t.Errorf("prune interval default = %v", feedItemsPruneInterval())
	}
	if contentArchiverTimeout() != 5*time.Second {
		t.Errorf("archiver timeout default = %v", contentArchiverTimeout())
	}
}

func TestFeedScraperConfigEnv(t *testing.T) {
	os.Setenv("FEED_SCRAPE_INTERVAL_HOURS", "3")
	os.Setenv("FEED_ITEMS_MAX_AGE_HOURS", "48")
	os.Setenv("FEED_ITEMS_PRUNE_INTERVAL_HOURS", "2")
	os.Setenv("CONTENT_ARCHIVER_TIMEOUT_SECONDS", "9")
	defer func() {
		for _, v := range []string{"FEED_SCRAPE_INTERVAL_HOURS", "FEED_ITEMS_MAX_AGE_HOURS", "FEED_ITEMS_PRUNE_INTERVAL_HOURS", "CONTENT_ARCHIVER_TIMEOUT_SECONDS"} {
			os.Unsetenv(v)
		}
	}()
	if feedScrapeInterval() != 3*time.Hour {
		t.Errorf("scrape interval = %v", feedScrapeInterval())
	}
	if feedItemsMaxAgeHours() != 48 {
		t.Errorf("max age = %d", feedItemsMaxAgeHours())
	}
	if feedItemsPruneInterval() != 2*time.Hour {
		t.Errorf("prune interval = %v", feedItemsPruneInterval())
	}
	if contentArchiverTimeout() != 9*time.Second {
		t.Errorf("archiver timeout = %v", contentArchiverTimeout())
	}
}

func TestScrapeFeed(t *testing.T) {
	resetDB(t)
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssSample))
	})

	svc.ScrapeFeed(srv.URL)

	items, err := svc.q.GetFeedArchiveItems(context.Background(), feedArchiveParams(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 feed items, got %d", len(items))
	}
	// Verify the parsed fields actually landed (not just row count). Items come
	// back newest-first; both rssSample items must be present with their content.
	byURL := map[string]string{}
	for _, it := range items {
		byURL[it.ItemUrl] = it.Title
	}
	if byURL["https://example.com/1"] != "Post One" {
		t.Errorf("item 1 title = %q, want Post One", byURL["https://example.com/1"])
	}
	if byURL["https://reddit.com/r/go/comments/abc"] != "Reddit Post" {
		t.Errorf("reddit item title = %q, want Reddit Post", byURL["https://reddit.com/r/go/comments/abc"])
	}

	// Scraping again inserts no new items (UNIQUE constraint).
	svc.ScrapeFeed(srv.URL)
	items2, _ := svc.q.GetFeedArchiveItems(context.Background(), feedArchiveParams(srv.URL))
	if len(items2) != len(items) {
		t.Errorf("duplicate scrape added items: %d -> %d", len(items), len(items2))
	}
}

func TestScrapeFeedError(t *testing.T) {
	resetDB(t)
	setProxyURL(t, "http://127.0.0.1:0/dead")
	// Should not panic; just logs and returns.
	svc.ScrapeFeed("http://127.0.0.1:0/deadfeed")
}

func TestScrapeAllFeeds(t *testing.T) {
	resetDB(t)
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssSample))
	})
	// No feeds -> no-op.
	svc.ScrapeAllFeeds()

	uid := createTestUser(t, "scrape@y.com")
	svc.q.InsertUserSavedFeed(context.Background(), savedFeedParams(uid, srv.URL))
	svc.ScrapeAllFeeds()

	items, _ := svc.q.GetFeedArchiveItems(context.Background(), feedArchiveParams(srv.URL))
	if len(items) == 0 {
		t.Errorf("scrapeAllFeeds did not insert items")
	}
}

func TestPruneFeedItems(t *testing.T) {
	resetDB(t)

	// A recent item survives the default (14-day) window.
	scrapeFeedInsert(t, "feedx", "recent")
	svc.PruneFeedItems()
	if total, _ := svc.q.CountFeedArchiveItems(context.Background(), "feedx"); total != 1 {
		t.Fatalf("recent item wrongly pruned: %d", total)
	}

	// An item older than the window IS deleted. Backdate its scraped_at and set a
	// 1-hour max age so the prune actually removes something.
	scrapeFeedInsert(t, "feedx", "old")
	if _, err := testDB.Exec(`UPDATE feed_items SET scraped_at = datetime('now','-30 days') WHERE item_url = 'old'`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FEED_ITEMS_MAX_AGE_HOURS", "1")

	svc.PruneFeedItems()

	total, _ := svc.q.CountFeedArchiveItems(context.Background(), "feedx")
	if total != 1 {
		t.Errorf("expected only the recent item to remain, got total=%d", total)
	}
	// The old one specifically must be gone, the recent one must remain.
	rows, _ := svc.q.GetFeedArchiveItems(context.Background(), feedArchiveParams("feedx"))
	for _, r := range rows {
		if r.ItemUrl == "old" {
			t.Errorf("old item should have been pruned")
		}
	}
}

func TestPollContentArchive(t *testing.T) {
	resetDB(t)

	// No feed items -> returns false.
	if svc.PollContentArchive() {
		t.Errorf("expected false with no items")
	}

	// Insert an item pointing to a working article server.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(articleHTML))
	})
	setProxyURL(t, srv.URL)
	scrapeFeedInsert(t, "feedA", "https://example.com/article-poll")

	if !svc.PollContentArchive() {
		t.Errorf("expected true after processing an item")
	}
	// The article should now be archived.
	if _, err := svc.q.GetArticleArchive(context.Background(), "https://example.com/article-poll"); err != nil {
		t.Errorf("article not archived: %v", err)
	}
}

func TestPollContentArchiveFetchFailure(t *testing.T) {
	resetDB(t)
	setProxyURL(t, "http://127.0.0.1:0/dead")
	scrapeFeedInsert(t, "feedB", "http://127.0.0.1:0/deadarticle")
	// Fetch fails -> marks failed, returns true.
	if !svc.PollContentArchive() {
		t.Errorf("expected true (item processed with failure)")
	}
}

func TestFetchReadableBackground(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(articleHTML))
	})
	setProxyURL(t, srv.URL)
	a, err := svc.fetchReadableBackground("https://example.com/bg")
	if err != nil {
		t.Fatal(err)
	}
	if a.Title == "" {
		t.Errorf("empty title")
	}

	setProxyURL(t, "http://127.0.0.1:0/dead")
	if _, err := svc.fetchReadableBackground("http://127.0.0.1:0/dead"); err == nil {
		t.Error("expected error")
	}
}
