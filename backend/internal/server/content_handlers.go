package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/adhamsalama/inkfeed-backend/db"
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

// feedArchiveSearchHandler full-text searches a feed's archived articles. Only
// users who have saved the feed may search it.
func (a *App) feedArchiveSearchHandler(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if feedURL == "" || query == "" {
		jsonError(w, "url and q parameters required", http.StatusBadRequest)
		return
	}
	userID := r.Context().Value(contextKey("userID")).(int64)
	saved, err := a.q.UserHasSavedFeed(r.Context(), db.UserHasSavedFeedParams{UserID: userID, Url: feedURL})
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !saved {
		jsonError(w, "feed is not in your saved feeds", http.StatusForbidden)
		return
	}
	limit := int64(20)
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	offset := int64(0)
	if v, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); err == nil && v >= 0 {
		offset = v
	}
	page, err := a.content.SearchFeedArchive(feedURL, query, limit, offset)
	if err != nil {
		jsonError(w, "failed to search archive", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
