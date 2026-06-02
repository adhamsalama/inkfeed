package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"time"

	"github.com/adhamsalama/inkfeed-backend/db"
	readability "github.com/go-shiori/go-readability"
)

func feedScrapeInterval() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("FEED_SCRAPE_INTERVAL_HOURS")); err == nil && v > 0 {
		return time.Duration(v) * time.Hour
	}
	return time.Hour
}

func feedItemsMaxAgeHours() int {
	if v, err := strconv.Atoi(os.Getenv("FEED_ITEMS_MAX_AGE_HOURS")); err == nil && v > 0 {
		return v
	}
	return 14 * 24
}

func feedItemsPruneInterval() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("FEED_ITEMS_PRUNE_INTERVAL_HOURS")); err == nil && v > 0 {
		return time.Duration(v) * time.Hour
	}
	return time.Hour
}

func startFeedItemsPruner() {
	go func() {
		pruneFeedItems()
		ticker := time.NewTicker(feedItemsPruneInterval())
		defer ticker.Stop()
		for range ticker.C {
			pruneFeedItems()
		}
	}()
}

func pruneFeedItems() {
	ctx := context.Background()
	hours := strconv.Itoa(feedItemsMaxAgeHours())
	result, err := queries.DeleteOldFeedItems(ctx, sql.NullString{String: hours, Valid: true})
	if err != nil {
		log.Printf("feed items pruner: error: %v", err)
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("feed items pruner: deleted %d rows older than %s hours", n, hours)
	}
}

func startFeedScraper() {
	go func() {
		interval := feedScrapeInterval()
		scrapeAllFeeds()
		log.Printf("feed scraper: next run in %s", interval)
		for range time.Tick(interval) {
			scrapeAllFeeds()
			log.Printf("feed scraper: next run in %s", interval)
		}
	}()
}

func scrapeAllFeeds() {
	ctx := context.Background()
	urls, err := queries.GetDistinctSavedFeedURLs(ctx)
	if err != nil {
		log.Printf("feed scraper: failed to get feed URLs: %v", err)
		return
	}
	if len(urls) == 0 {
		return
	}
	log.Printf("feed scraper: scraping %d feeds", len(urls))
	for _, feedURL := range urls {
		scrapeFeed(feedURL)
	}
}

func scrapeFeed(feedURL string) {
	resp, err := fetchAndParseFeed(feedURL)
	if err != nil {
		log.Printf("feed scraper: failed to fetch %s: %v", feedURL, err)
		return
	}

	feedTitle := resp.Title
	if feedTitle == "" {
		feedTitle = feedURL
	}
	log.Printf("feed scraper: scraping %q (%d items)", feedTitle, len(resp.Articles))

	ctx := context.Background()
	newCount := 0
	for _, article := range resp.Articles {
		if article.Link == "" {
			continue
		}
		desc := article.Description
		if desc == "" {
			desc = article.Content
		}
		pubDate := article.PubDate
		if t, err := time.Parse(time.RFC1123, pubDate); err == nil {
			pubDate = t.UTC().Format(time.RFC3339)
		} else if t, err := time.Parse(time.RFC1123Z, pubDate); err == nil {
			pubDate = t.UTC().Format(time.RFC3339)
		}
		commentsUrl := sql.NullString{String: article.Comments, Valid: article.Comments != ""}
		res, err := queries.InsertFeedItem(ctx, db.InsertFeedItemParams{
			FeedUrl:     feedURL,
			ItemUrl:     article.Link,
			Title:       article.Title,
			Description: desc,
			PubDate:     pubDate,
			CommentsUrl: commentsUrl,
		})
		if err != nil {
			log.Printf("feed scraper: insert error for %s: %v", article.Link, err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("feed scraper: new item %q", article.Title)
			newCount++
		}
	}
	log.Printf("feed scraper: done %q — %d new, %d already seen", feedTitle, newCount, len(resp.Articles)-newCount)
}

// startContentArchiver polls for feed items that haven't been fully archived yet
// and fetches their article content in the background.
func startContentArchiver() {
	go func() {
		for {
			if pollContentArchive() {
				time.Sleep(2 * time.Second)
			} else {
				time.Sleep(5 * time.Second)
			}
		}
	}()
}

func contentArchiverTimeout() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("CONTENT_ARCHIVER_TIMEOUT_SECONDS")); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return 5 * time.Second
}

// fetchReadableBackground fetches an article with a short timeout, falling
// back to the proxy on error — suitable for best-effort background archiving.
func fetchReadableBackground(rawURL string) (readability.Article, error) {
	client := newScrapingClient(ScrapingClientConfig{Timeout: contentArchiverTimeout(), WithProxy: true, UseProxyFirst: true})
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return readability.Article{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return readability.Article{}, err
	}
	defer resp.Body.Close()
	parsedURL, _ := neturl.Parse(rawURL)
	return readability.FromReader(resp.Body, parsedURL)
}

func pollContentArchive() bool {
	ctx := context.Background()
	itemURL, err := queries.GetNextFeedItemWithoutArchive(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("content archiver query error: %v", err)
		}
		return false
	}

	article, err := fetchReadableBackground(itemURL)
	if err != nil {
		log.Printf("content archiver: skipping %s: %v", itemURL, err)
		if err := queries.MarkFeedItemArchiveFailed(ctx, itemURL); err != nil {
			log.Printf("content archiver: failed to mark %s as failed: %v", itemURL, err)
		}
		return true
	}

	publishedTime := ""
	if article.PublishedTime != nil {
		publishedTime = article.PublishedTime.Format("2 January 2006")
	}
	archiveArticle(itemURL, article.Title, article.Byline, article.SiteName, publishedTime, article.Content, article.TextContent)

	log.Printf("content archiver: archived %s", itemURL)
	return true
}

func feedArchiveHandler(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	if feedURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}

	limit := int64(50)
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	offset := int64(0)
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); err == nil && v >= 0 {
		offset = v
	}

	ctx := context.Background()
	rows, err := queries.GetFeedArchiveItems(ctx, db.GetFeedArchiveItemsParams{
		FeedUrl: feedURL,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		jsonError(w, "failed to query archive", http.StatusInternalServerError)
		return
	}

	total, err := queries.CountFeedArchiveItems(ctx, feedURL)
	if err != nil {
		total = 0
	}

	type archiveArticle struct {
		Index       int    `json:"index"`
		Title       string `json:"title"`
		Link        string `json:"link"`
		Description string `json:"description"`
		PubDate     string `json:"pubDate"`
		Comments    string `json:"comments"`
	}

	articles := make([]archiveArticle, len(rows))
	for i, row := range rows {
		comments := row.CommentsUrl.String
		articles[i] = archiveArticle{
			Index:       int(offset) + i,
			Title:       row.Title,
			Link:        row.ItemUrl,
			Description: row.Description,
			PubDate:     row.PubDate,
			Comments:    comments,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"articles": articles,
		"total":    total,
		"hasMore":  offset+int64(len(rows)) < total,
	})
}
