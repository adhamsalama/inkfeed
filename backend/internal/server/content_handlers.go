package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/adhamsalama/inkfeed-backend/internal/content"
	"github.com/adhamsalama/inkfeed-backend/internal/export"
)

// These handlers are thin HTTP adapters over the content.Service capability:
// parse the request, call the service, write the response.

func (a *App) feedHandler(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	if feedURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	resp, err := a.content.FetchAndParseFeed(feedURL)
	if err != nil {
		jsonError(w, "failed to parse feed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(resp)
}

func (a *App) textHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	article, err := a.content.FetchReadable(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	filename := export.Filename(article.Title, "txt", false)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write([]byte(article.TextContent))
}

func (a *App) articleHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	resp, err := a.content.Article(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(resp)
}

func (a *App) commentsHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	htmlContent, err := a.content.Comments(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(map[string]string{"html": htmlContent})
}

func (a *App) redditPostHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	resp, err := a.content.RedditPost(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (a *App) decodeGoogleNewsHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}
	decoded, err := content.DecodeGoogleNewsURL(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"decoded_url": decoded})
}

func (a *App) feedArchiveHandler(w http.ResponseWriter, r *http.Request) {
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
	page, err := a.content.FeedArchive(feedURL, limit, offset)
	if err != nil {
		jsonError(w, "failed to query archive", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

// maxSearchFeeds caps how many feeds one search request may span.
const maxSearchFeeds = 200

// feedArchiveSearchHandler full-text searches the archived articles of one or
// more feeds (repeated url params, e.g. a feed group). Every feed must be in the
// user's saved feeds or feed groups.
func (a *App) feedArchiveSearchHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var feedURLs []string
	seen := make(map[string]bool)
	for _, u := range r.URL.Query()["url"] {
		if u != "" && !seen[u] {
			seen[u] = true
			feedURLs = append(feedURLs, u)
		}
	}
	if len(feedURLs) == 0 || query == "" {
		jsonError(w, "url and q parameters required", http.StatusBadRequest)
		return
	}
	if len(feedURLs) > maxSearchFeeds {
		jsonError(w, "too many feeds", http.StatusBadRequest)
		return
	}
	userID := r.Context().Value(contextKey("userID")).(int64)
	allowed, err := a.q.GetUserSearchableFeedURLs(r.Context(), userID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, u := range allowed {
		allowedSet[u] = true
	}
	for _, u := range feedURLs {
		if !allowedSet[u] {
			jsonError(w, "feed is not in your saved feeds or groups", http.StatusForbidden)
			return
		}
	}
	limit := int64(20)
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	offset := int64(0)
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); err == nil && v >= 0 {
		offset = v
	}
	page, err := a.content.SearchFeedArchive(feedURLs, query, limit, offset)
	if err != nil {
		jsonError(w, "failed to search archive", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
