package content

import (
	"context"
	"database/sql"
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

func (s *Service) StartFeedItemsPruner() {
	go func() {
		s.PruneFeedItems()
		ticker := time.NewTicker(feedItemsPruneInterval())
		defer ticker.Stop()
		for range ticker.C {
			s.PruneFeedItems()
		}
	}()
}

func (s *Service) PruneFeedItems() {
	ctx := context.Background()
	hours := strconv.Itoa(feedItemsMaxAgeHours())
	result, err := s.q.DeleteOldFeedItems(ctx, sql.NullString{String: hours, Valid: true})
	if err != nil {
		log.Printf("feed items pruner: error: %v", err)
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("feed items pruner: deleted %d rows older than %s hours", n, hours)
	}
}

func (s *Service) StartFeedScraper() {
	go func() {
		interval := feedScrapeInterval()
		s.ScrapeAllFeeds()
		log.Printf("feed scraper: next run in %s", interval)
		for range time.Tick(interval) {
			s.ScrapeAllFeeds()
			log.Printf("feed scraper: next run in %s", interval)
		}
	}()
}

func (s *Service) ScrapeAllFeeds() {
	ctx := context.Background()
	urls, err := s.q.GetDistinctSavedFeedURLs(ctx)
	if err != nil {
		log.Printf("feed scraper: failed to get feed URLs: %v", err)
		return
	}
	if len(urls) == 0 {
		return
	}
	log.Printf("feed scraper: scraping %d feeds", len(urls))
	for _, feedURL := range urls {
		s.ScrapeFeed(feedURL)
	}
}

func (s *Service) ScrapeFeed(feedURL string) {
	resp, err := s.FetchAndParseFeed(feedURL)
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
		res, err := s.q.InsertFeedItem(ctx, db.InsertFeedItemParams{
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

// StartContentArchiver polls for feed items that haven't been fully archived yet
// and fetches their article content in the background.
func (s *Service) StartContentArchiver() {
	go func() {
		for {
			if s.PollContentArchive() {
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
func (s *Service) fetchReadableBackground(rawURL string) (readability.Article, error) {
	client := s.newClient(ScrappingClientConfig{Timeout: contentArchiverTimeout(), WithProxy: true, UseProxyFirst: true})
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

func (s *Service) PollContentArchive() bool {
	ctx := context.Background()
	itemURL, err := s.q.GetNextFeedItemWithoutArchive(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("content archiver query error: %v", err)
		}
		return false
	}

	article, err := s.fetchReadableBackground(itemURL)
	if err != nil {
		log.Printf("content archiver: skipping %s: %v", itemURL, err)
		if err := s.q.MarkFeedItemArchiveFailed(ctx, itemURL); err != nil {
			log.Printf("content archiver: failed to mark %s as failed: %v", itemURL, err)
		}
		return true
	}

	publishedTime := ""
	if article.PublishedTime != nil {
		publishedTime = article.PublishedTime.Format("2 January 2006")
	}
	s.ArchiveArticle(itemURL, article.Title, article.Byline, article.SiteName, publishedTime, article.Content, article.TextContent)

	log.Printf("content archiver: archived %s", itemURL)
	return true
}

// ArchiveArticle is one item in a paginated feed-archive page.
type ArchiveArticle struct {
	Index       int    `json:"index"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
	PubDate     string `json:"pubDate"`
	Comments    string `json:"comments"`
}

// FeedArchivePage is a paginated slice of a feed's archived items.
type FeedArchivePage struct {
	Articles []ArchiveArticle `json:"articles"`
	Total    int64            `json:"total"`
	HasMore  bool             `json:"hasMore"`
}

// FeedArchive returns a page of archived items for the given feed URL.
func (s *Service) FeedArchive(feedURL string, limit, offset int64) (FeedArchivePage, error) {
	ctx := context.Background()
	rows, err := s.q.GetFeedArchiveItems(ctx, db.GetFeedArchiveItemsParams{
		FeedUrl: feedURL,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return FeedArchivePage{}, err
	}

	total, err := s.q.CountFeedArchiveItems(ctx, feedURL)
	if err != nil {
		total = 0
	}

	articles := make([]ArchiveArticle, len(rows))
	for i, row := range rows {
		articles[i] = ArchiveArticle{
			Index:       int(offset) + i,
			Title:       row.Title,
			Link:        row.ItemUrl,
			Description: row.Description,
			PubDate:     row.PubDate,
			Comments:    row.CommentsUrl.String,
		}
	}

	return FeedArchivePage{
		Articles: articles,
		Total:    total,
		HasMore:  offset+int64(len(rows)) < total,
	}, nil
}
