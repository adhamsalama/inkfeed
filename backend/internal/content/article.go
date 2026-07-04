package content

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/adhamsalama/inkfeed-backend/db"
	readability "github.com/go-shiori/go-readability"
)

type ArticleResponse struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	Byline        string `json:"byline"`
	SiteName      string `json:"siteName"`
	PublishedTime string `json:"publishedTime"`
	WordCount     int    `json:"wordCount"`
}

// Article returns the readable article for rawURL, served from the archive cache
// when present. On a cache miss it fetches with Readability, then archives the
// result in the background.
func (s *Service) Article(rawURL string) (ArticleResponse, error) {
	if row, err := s.q.GetArticleArchive(context.Background(), rawURL); err == nil {
		log.Printf("cache hit (archive): %s", rawURL)
		return ArticleResponse{
			Title:         row.Title,
			Content:       row.HtmlContent,
			Byline:        row.Author,
			SiteName:      row.SiteName,
			PublishedTime: row.CreatedAt,
			WordCount:     len(strings.Fields(row.TextContent)),
		}, nil
	}

	article, err := s.FetchReadable(rawURL)
	if err != nil {
		return ArticleResponse{}, err
	}

	publishedTime := ""
	if article.PublishedTime != nil {
		publishedTime = article.PublishedTime.Format("2 January 2006")
	}
	resp := ArticleResponse{
		Title:         article.Title,
		Content:       article.Content,
		Byline:        article.Byline,
		SiteName:      article.SiteName,
		PublishedTime: publishedTime,
		WordCount:     len(strings.Fields(article.TextContent)),
	}
	go s.ArchiveArticle(rawURL, article.Title, article.Byline, article.SiteName, publishedTime, article.Content, article.TextContent)
	return resp, nil
}

func (s *Service) ArchiveArticle(key, title, author, siteName, createdAt, htmlContent, textContent string) {
	ctx := context.Background()
	log.Printf("article archived: %s", key)
	if err := s.q.UpsertArticleArchive(ctx, db.UpsertArticleArchiveParams{
		Key:         key,
		Title:       title,
		Author:      author,
		SiteName:    siteName,
		CreatedAt:   createdAt,
		HtmlContent: htmlContent,
		TextContent: textContent,
	}); err != nil {
		log.Printf("article archive write error: %v", err)
	}
}

const archivePruneTargetBytes = 90 * 1024 * 1024 // 90 MB - prune down to this

func (s *Service) PruneArticleArchive() {
	ctx := context.Background()
	size, err := s.q.GetArticleArchiveTotalSize(ctx)
	if err != nil {
		log.Printf("article archive size check error: %v", err)
		return
	}
	if size <= archivePruneTargetBytes {
		return
	}
	log.Printf("article archive size %d bytes exceeds target, pruning oldest articles", size)
	deleted := 0
	for size > archivePruneTargetBytes {
		article, err := s.q.GetOldestArticleArchiveKey(ctx)
		if err != nil {
			log.Printf("article archive prune error: %v", err)
			return
		}
		if err := s.q.DeleteOldestArticleArchiveRow(ctx); err != nil {
			log.Printf("article archive delete error: %v", err)
			return
		}
		deleted++
		log.Printf("article archive deleted: %s (%s)", article.Key, article.Title)
		size, err = s.q.GetArticleArchiveTotalSize(ctx)
		if err != nil {
			log.Printf("article archive size check error: %v", err)
			return
		}
	}
	log.Printf("article archive pruned %d rows, size now %d bytes", deleted, size)
}

func (s *Service) StartArticleArchivePruner() {
	go func() {
		s.PruneArticleArchive() // run once at startup
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.PruneArticleArchive()
		}
	}()
}

// ArticleMeta returns an HTML snippet with article metadata.
func ArticleMeta(article readability.Article) string {
	var sb strings.Builder

	// "author @ sitename" line
	byline := strings.TrimSpace(article.Byline)
	siteName := strings.TrimSpace(article.SiteName)
	if byline != "" && siteName != "" {
		sb.WriteString(`<p><em>` + html.EscapeString(byline) + ` @ ` + html.EscapeString(siteName) + `</em></p>`)
	} else if byline != "" {
		sb.WriteString(`<p><em>` + html.EscapeString(byline) + `</em></p>`)
	} else if siteName != "" {
		sb.WriteString(`<p><em>` + html.EscapeString(siteName) + `</em></p>`)
	}

	// reading time (avg 200 wpm)
	wordCount := len(strings.Fields(article.TextContent))
	if wordCount > 0 {
		minutes := wordCount / 200
		if minutes < 1 {
			minutes = 1
		}
		sb.WriteString(`<p><em>` + fmt.Sprintf("%d min read", minutes) + `</em></p>`)
	}

	// published date
	if article.PublishedTime != nil {
		sb.WriteString(`<p><em>Published:` + article.PublishedTime.Format("2 January 2006") + `</em></p>`)
	}

	if sb.Len() > 0 {
		sb.WriteString("<hr/>")
	}
	return sb.String()
}

// FetchReadable fetches a URL and runs Mozilla Readability on the response.
func (s *Service) FetchReadable(rawURL string) (readability.Article, error) {
	client := s.newClient(ScrappingClientConfig{Timeout: 30 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return readability.Article{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return readability.Article{}, err
	}
	defer resp.Body.Close()
	parsedURL, _ := url.Parse(rawURL)
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return article, err
	}
	article.Content = dedupeResponsiveImages(article.Content)
	return article, nil
}

var (
	imgTagRe = regexp.MustCompile(`(?i)<img\s[^>]*>`)
	imgSrcRe = regexp.MustCompile(`(?i)\bsrc="([^"]*)"`)
)

// ResponsiveVariantKey returns a normalized key identifying a responsive image
// variant, or "" if the URL carries no responsive marker (so it never
// participates in dedup). It strips the "desktop"/"mobile" tokens so the two
// variants of the same figure collapse to the same key. Shared with the MOBI
// image-embedding path in the export package.
func ResponsiveVariantKey(u string) string {
	lower := strings.ToLower(u)
	if !strings.Contains(lower, "desktop") && !strings.Contains(lower, "mobile") {
		return ""
	}
	return strings.NewReplacer("desktop", "", "mobile", "").Replace(lower)
}

// dedupeResponsiveImages removes duplicate responsive <img> variants (keeping
// the first) from extracted article HTML. Sites like Quanta ship separate
// "Desktop" and "Mobile" copies of the same figure that CSS media queries hide;
// without CSS both would render, so collapse variants that differ only by the
// desktop/mobile token.
func dedupeResponsiveImages(htmlContent string) string {
	seen := map[string]bool{}
	return imgTagRe.ReplaceAllStringFunc(htmlContent, func(imgTag string) string {
		m := imgSrcRe.FindStringSubmatch(imgTag)
		if len(m) < 2 {
			return imgTag
		}
		key := ResponsiveVariantKey(m[1])
		if key == "" {
			return imgTag
		}
		if seen[key] {
			return ""
		}
		seen[key] = true
		return imgTag
	})
}
