package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const rssSample = `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
  <title>Example Feed</title>
  <item>
    <title>Post One</title>
    <link>https://example.com/1</link>
    <description>First post</description>
    <comments>https://example.com/1/comments</comments>
    <pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate>
    <content:encoded><![CDATA[<p>Full content</p>]]></content:encoded>
  </item>
  <item>
    <title>Reddit Post</title>
    <link>https://reddit.com/r/go/comments/abc</link>
    <description>Reddit</description>
  </item>
</channel>
</rss>`

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <title>Atom Entry</title>
    <link href="https://example.com/atom1"/>
    <summary>Atom summary</summary>
    <published>2006-01-02T15:04:05Z</published>
  </entry>
</feed>`

func TestParseFeedRSS(t *testing.T) {
	resp, err := parseFeed("https://example.com/feed", []byte(rssSample))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Example Feed" {
		t.Errorf("title = %q", resp.Title)
	}
	if len(resp.Articles) != 2 {
		t.Fatalf("articles = %d", len(resp.Articles))
	}
	if resp.Articles[0].Comments != "https://example.com/1/comments" {
		t.Errorf("comments = %q", resp.Articles[0].Comments)
	}
	if resp.Articles[0].PubDate == "" {
		t.Errorf("pubdate empty")
	}
	// Reddit link gets /.json comments appended.
	if resp.Articles[1].Comments != "https://reddit.com/r/go/comments/abc/.json" {
		t.Errorf("reddit comments = %q", resp.Articles[1].Comments)
	}
}

func TestParseFeedAtom(t *testing.T) {
	resp, err := parseFeed("https://example.com/atom", []byte(atomSample))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Atom Feed" {
		t.Errorf("title = %q", resp.Title)
	}
	if len(resp.Articles) != 1 || resp.Articles[0].Title != "Atom Entry" {
		t.Fatalf("atom articles = %+v", resp.Articles)
	}
	if resp.Articles[0].PubDate == "" {
		t.Errorf("atom pubdate empty")
	}
}

func TestParseFeedInvalid(t *testing.T) {
	if _, err := parseFeed("https://x", []byte("not a feed at all")); err == nil {
		t.Error("expected error for garbage input")
	}
}

func TestFetchAndParseFeedDirect(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssSample))
	})
	resp, err := app.fetchAndParseFeed(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Example Feed" {
		t.Errorf("title = %q", resp.Title)
	}
}

func TestFetchAndParseFeedProxyFallback(t *testing.T) {
	// Direct URL is unreachable; proxy serves the feed.
	proxy := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssSample))
	})
	setProxyURL(t, proxy.URL)
	// Use a syntactically valid but unroutable direct URL.
	resp, err := app.fetchAndParseFeed("http://127.0.0.1:0/nope")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Title != "Example Feed" {
		t.Errorf("proxy fallback title = %q", resp.Title)
	}
}

func TestFeedHandler(t *testing.T) {
	// Missing url param
	w := httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssSample))
	})
	w = httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed?url="+srv.URL, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("feed = %d", w.Code)
	}
	var resp FeedResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Title != "Example Feed" {
		t.Errorf("handler title = %q", resp.Title)
	}
}

func TestFeedHandlerError(t *testing.T) {
	setProxyURL(t, "http://127.0.0.1:0/x")
	w := httptest.NewRecorder()
	app.feedHandler(w, httptest.NewRequest(http.MethodGet, "/feed?url=http://127.0.0.1:0/bad", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("feed error = %d", w.Code)
	}
}
