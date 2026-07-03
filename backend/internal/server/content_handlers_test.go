package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adhamsalama/inkfeed-backend/db"
)

// rssSample is a minimal RSS feed used by the feed handler test.
const rssSample = `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Feed</title>
<item><title>Post One</title><link>https://example.com/1</link><description>d</description></item>
</channel></rss>`

func TestFeedHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(rssSample)) })
	w = httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed?url="+srv.URL, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("feed = %d", w.Code)
	}
	var feed struct {
		Title    string `json:"title"`
		Articles []struct {
			Title string `json:"title"`
			Link  string `json:"link"`
		} `json:"articles"`
	}
	mustJSON(t, w, &feed)
	if feed.Title != "Example Feed" {
		t.Errorf("feed title = %q", feed.Title)
	}
	if len(feed.Articles) != 1 || feed.Articles[0].Title != "Post One" || feed.Articles[0].Link != "https://example.com/1" {
		t.Errorf("feed articles = %+v", feed.Articles)
	}

	setProxyURL(t, "http://127.0.0.1:0/x")
	w = httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed?url=http://127.0.0.1:0/bad", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("feed error = %d", w.Code)
	}
}

func TestTextHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.textHandler(w, httptest.NewRequest(http.MethodGet, "/text", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	app.textHandler(w, httptest.NewRequest(http.MethodGet, "/text?url=https://example.com/p", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("text = %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), ".txt") {
		t.Errorf("disposition = %q", w.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(w.Body.String(), "long paragraph") {
		t.Errorf("text body missing extracted content: %q", w.Body.String())
	}

	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.textHandler(w, httptest.NewRequest(http.MethodGet, "/text?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("text error = %d", w.Code)
	}
}

func TestArticleHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.articleHandler(w, httptest.NewRequest(http.MethodGet, "/article", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	app.articleHandler(w, httptest.NewRequest(http.MethodGet, "/article?url=https://example.com/a", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("article = %d", w.Code)
	}
	var art struct {
		Title     string `json:"title"`
		Content   string `json:"content"`
		WordCount int    `json:"wordCount"`
	}
	mustJSON(t, w, &art)
	if !strings.Contains(art.Title, "Test Article Title") {
		t.Errorf("article title = %q", art.Title)
	}
	if art.Content == "" || art.WordCount == 0 {
		t.Errorf("article content/wordcount empty: %+v", art)
	}

	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.articleHandler(w, httptest.NewRequest(http.MethodGet, "/article?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("article error = %d", w.Code)
	}
}

func TestCommentsHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	// HN comments route
	hnSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"children":[{"id":2,"author":"alice","text":"first post","children":[]}]}`))
	})
	setProxyURL(t, hnSrv.URL)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://news.ycombinator.com/item?id=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("comments = %d", w.Code)
	}
	var cr struct {
		HTML string `json:"html"`
	}
	mustJSON(t, w, &cr)
	if !strings.Contains(cr.HTML, "alice") || !strings.Contains(cr.HTML, "first post") {
		t.Errorf("comments html missing rendered comment: %q", cr.HTML)
	}

	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("comments error = %d", w.Code)
	}
}

func TestRedditPostHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"data":{"children":[{"data":{"is_self":true,"selftext":"hi there"}}]}},{"data":{"children":[]}}]`))
	})
	setProxyURL(t, srv.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/x/.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("reddit = %d", w.Code)
	}
	var rp struct {
		ContentHTML string `json:"content_html"`
	}
	mustJSON(t, w, &rp)
	if rp.ContentHTML != "<p>hi there</p>" {
		t.Errorf("reddit content = %q", rp.ContentHTML)
	}

	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("reddit error = %d", w.Code)
	}
}

func TestDecodeGoogleNewsHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.decodeGoogleNewsHandler(w, httptest.NewRequest(http.MethodGet, "/decode-google-news", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	w = httptest.NewRecorder()
	app.decodeGoogleNewsHandler(w, httptest.NewRequest(http.MethodGet, "/decode-google-news?url=https://example.com/x", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("bad url = %d", w.Code)
	}
}

func TestFeedArchiveHandler(t *testing.T) {
	resetDB(t)

	w := httptest.NewRecorder()
	app.feedArchiveHandler(w, httptest.NewRequest(http.MethodGet, "/feed-archive", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	// Seed one item (content.Service shares app's DB handle).
	app.q.InsertFeedItem(context.Background(), db.InsertFeedItemParams{
		FeedUrl: "feedZ", ItemUrl: "https://example.com/z1", Title: "Z One", Description: "d", PubDate: "2024-01-01T00:00:00Z",
	})
	w = httptest.NewRecorder()
	app.feedArchiveHandler(w, httptest.NewRequest(http.MethodGet, "/feed-archive?url=feedZ&limit=10&offset=0", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("archive = %d", w.Code)
	}
	var page struct {
		Total    int `json:"total"`
		Articles []struct {
			Title string `json:"title"`
			Link  string `json:"link"`
		} `json:"articles"`
	}
	mustJSON(t, w, &page)
	if page.Total != 1 || len(page.Articles) != 1 || page.Articles[0].Title != "Z One" {
		t.Errorf("archive page = %+v", page)
	}
}

func mustJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q, want json", ct)
	}
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
}
