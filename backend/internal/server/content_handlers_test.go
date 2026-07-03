package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
	if w.Code != http.StatusOK || !contains(w.Header().Get("Content-Disposition"), ".txt") {
		t.Errorf("text = %d disp=%q", w.Code, w.Header().Get("Content-Disposition"))
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

	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://example.com/g", nil))
	if w.Code != http.StatusOK {
		t.Errorf("comments = %d", w.Code)
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
		w.Write([]byte(`[{"data":{"children":[{"data":{"is_self":true,"selftext":"hi"}}]}},{"data":{"children":[]}}]`))
	})
	setProxyURL(t, srv.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/x/.json", nil))
	if w.Code != http.StatusOK {
		t.Errorf("reddit = %d", w.Code)
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

	w = httptest.NewRecorder()
	app.feedArchiveHandler(w, httptest.NewRequest(http.MethodGet, "/feed-archive?url=feedZ&limit=10&offset=0", nil))
	if w.Code != http.StatusOK {
		t.Errorf("archive = %d", w.Code)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
