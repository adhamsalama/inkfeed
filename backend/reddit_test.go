package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedditPostHandler(t *testing.T) {
	// missing url
	w := httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	// self post with selftext_html
	selfJSON := `[{"data":{"children":[{"data":{"is_self":true,"selftext_html":"&lt;!-- SC_OFF --&gt;&lt;div class=\"md\"&gt;&lt;p&gt;body text&lt;/p&gt;&lt;/div&gt;&lt;!-- SC_ON --&gt;"}}]}},{"data":{"children":[]}}]`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(selfJSON))
	})
	setProxyURL(t, srv.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/x/.json", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("self post = %d", w.Code)
	}
	var resp RedditPostResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ContentHTML != "<p>body text</p>" {
		t.Errorf("content = %q", resp.ContentHTML)
	}

	// link post with plain selftext
	linkJSON := `[{"data":{"children":[{"data":{"is_self":false,"url":"https://target.com/article","selftext":"some text"}}]}},{"data":{"children":[]}}]`
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(linkJSON))
	})
	setProxyURL(t, srv2.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/y/.json", nil))
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ActualURL != "https://target.com/article" {
		t.Errorf("actual url = %q", resp.ActualURL)
	}
	if resp.ContentHTML != "<p>some text</p>" {
		t.Errorf("selftext content = %q", resp.ContentHTML)
	}

	// bad JSON
	srv3 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})
	setProxyURL(t, srv3.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/z/.json", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("bad json = %d", w.Code)
	}

	// unexpected structure (empty children)
	srv4 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"data":{"children":[]}}]`))
	})
	setProxyURL(t, srv4.URL)
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=https://reddit.com/w/.json", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("empty children = %d", w.Code)
	}

	// fetch error
	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.redditPostHandler(w, httptest.NewRequest(http.MethodGet, "/reddit-post?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("fetch error = %d", w.Code)
	}
}
