package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
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

func textHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}

	article, err := fetchReadableWithFallback(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	filename := sanitizeFilename(article.Title) + ".txt"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write([]byte(article.TextContent))
}

func fetchReadableWithFallback(rawURL string) (readability.Article, error) {
	article, err := fetchReadable(rawURL)
	if err != nil || article.Title == "Just a moment..." {
		if err == nil {
			log.Printf("cloudflare challenge detected for %s, retrying via proxy", rawURL)
		}
		proxyURL := feedProxyURL + "?url=" + rawURL
		return fetchReadable(proxyURL)
	}
	return article, nil
}

func articleHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}

	if row, err := queries.GetArticleArchive(r.Context(), rawURL); err == nil {
		log.Printf("cache hit (archive): %s", rawURL)
		resp := ArticleResponse{
			Title:         row.Title,
			Content:       row.HtmlContent,
			Byline:        row.Author,
			SiteName:      row.SiteName,
			PublishedTime: row.CreatedAt,
			WordCount:     len(strings.Fields(row.TextContent)),
		}
		body, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(body)
		return
	}

	article, err := fetchReadableWithFallback(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
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
	body, err := json.Marshal(resp)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	go archiveArticle(rawURL, article.Title, article.Byline, article.SiteName, publishedTime, article.Content, article.TextContent)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(body)
}

func archiveArticle(key, title, author, siteName, createdAt, htmlContent, textContent string) {
	ctx := context.Background()
	log.Printf("article archived: %s", key)
	if err := queries.UpsertArticleArchive(ctx, db.UpsertArticleArchiveParams{
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

func pruneArticleArchive() {
	ctx := context.Background()
	size, err := queries.GetArticleArchiveTotalSize(ctx)
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
		article, err := queries.GetOldestArticleArchiveKey(ctx)
		if err != nil {
			log.Printf("article archive prune error: %v", err)
			return
		}
		if err := queries.DeleteOldestArticleArchiveRow(ctx); err != nil {
			log.Printf("article archive delete error: %v", err)
			return
		}
		deleted++
		log.Printf("article archive deleted: %s (%s)", article.Key, article.Title)
		size, err = queries.GetArticleArchiveTotalSize(ctx)
		if err != nil {
			log.Printf("article archive size check error: %v", err)
			return
		}
	}
	log.Printf("article archive pruned %d rows, size now %d bytes", deleted, size)
}

func startArticleArchivePruner() {
	go func() {
		pruneArticleArchive() // run once at startup
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			pruneArticleArchive()
		}
	}()
}

// articleMetaHTML returns an HTML snippet with article metadata.
func articleMetaHTML(article readability.Article) string {
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

// fetchReadable fetches a URL and runs Mozilla Readability on the response.
func fetchReadable(rawURL string) (readability.Article, error) {
	client := newScrapingClient(ScrapingClientConfig{Timeout: 30 * time.Second, WithProxy: true, UseProxyFirst: true})
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
	return readability.FromReader(resp.Body, parsedURL)
}
