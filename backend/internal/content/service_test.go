package content

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestResponsiveVariantKey(t *testing.T) {
	if ResponsiveVariantKey("https://x/photo.jpg") != "" {
		t.Errorf("no marker should be empty")
	}
	d := ResponsiveVariantKey("https://x/photo-Desktop.jpg")
	m := ResponsiveVariantKey("https://x/photo-Mobile.jpg")
	if d == "" || d != m {
		t.Errorf("desktop/mobile should collapse: %q vs %q", d, m)
	}
}

func TestArticle(t *testing.T) {
	resetDB(t)

	// cache miss -> fetch + background archive
	serveArticleViaProxy(t, articleHTML)
	resp, err := svc.Article("https://example.com/a1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Title, "Test Article Title") {
		t.Errorf("title = %q", resp.Title)
	}

	// cache hit (insert directly)
	svc.ArchiveArticle("https://example.com/cached", "Cached", "Au", "Site", "2024", "<p>x</p>", "x y z")
	resp, err = svc.Article("https://example.com/cached")
	if err != nil || resp.Title != "Cached" {
		t.Fatalf("cache hit: %v %+v", err, resp)
	}

	// error path
	setProxyURL(t, "http://127.0.0.1:0/dead")
	if _, err := svc.Article("http://127.0.0.1:0/dead"); err == nil {
		t.Error("expected error")
	}
}

func TestComments(t *testing.T) {
	// success: HN route returns rendered comment HTML
	hn := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"children":[{"id":2,"author":"bob","text":"hello there","children":[]}]}`))
	})
	setProxyURL(t, hn.URL)
	out, err := svc.Comments("https://news.ycombinator.com/item?id=1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bob") || !strings.Contains(out, "hello there") {
		t.Errorf("comments html missing rendered content: %q", out)
	}

	// error path (unreachable generic URL, both proxy+direct dead)
	setProxyURL(t, "http://127.0.0.1:0/dead")
	if _, err := svc.Comments("http://127.0.0.1:0/dead-generic"); err == nil {
		t.Error("expected error")
	}
}

func TestRedditPost(t *testing.T) {
	// self post
	selfJSON := `[{"data":{"children":[{"data":{"is_self":true,"selftext_html":"&lt;!-- SC_OFF --&gt;&lt;div class=\"md\"&gt;&lt;p&gt;body text&lt;/p&gt;&lt;/div&gt;&lt;!-- SC_ON --&gt;"}}]}},{"data":{"children":[]}}]`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(selfJSON)) })
	setProxyURL(t, srv.URL)
	resp, err := svc.RedditPost("https://reddit.com/x/.json")
	if err != nil || resp.ContentHTML != "<p>body text</p>" {
		t.Fatalf("self post: %v %+v", err, resp)
	}

	// link post
	linkJSON := `[{"data":{"children":[{"data":{"is_self":false,"url":"https://t.com/a","selftext":"plain"}}]}},{"data":{"children":[]}}]`
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(linkJSON)) })
	setProxyURL(t, srv2.URL)
	resp, _ = svc.RedditPost("https://reddit.com/y/.json")
	if resp.ActualURL != "https://t.com/a" || resp.ContentHTML != "<p>plain</p>" {
		t.Errorf("link post = %+v", resp)
	}

	// bad json
	srv3 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("nope")) })
	setProxyURL(t, srv3.URL)
	if _, err := svc.RedditPost("https://reddit.com/z/.json"); err == nil {
		t.Error("expected parse error")
	}

	// unexpected structure
	srv4 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[{"data":{"children":[]}}]`)) })
	setProxyURL(t, srv4.URL)
	if _, err := svc.RedditPost("https://reddit.com/w/.json"); err == nil {
		t.Error("expected structure error")
	}

	// fetch error
	setProxyURL(t, "http://127.0.0.1:0/dead")
	if _, err := svc.RedditPost("http://127.0.0.1:0/dead"); err == nil {
		t.Error("expected fetch error")
	}
}

func TestFeedArchive(t *testing.T) {
	resetDB(t)
	scrapeFeedInsert(t, "feedH", "https://example.com/h1")
	page, err := svc.FeedArchive("feedH", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Articles) != 1 {
		t.Errorf("page = %+v", page)
	}
}

func TestPruneArticleArchiveLoop(t *testing.T) {
	resetDB(t)
	big := strings.Repeat("A", 20*1024*1024) // 20MB per field
	for i := range 3 {
		svc.ArchiveArticle("bigkey-"+string(rune('a'+i)), "T", "au", "s", "2024", big, big)
	}
	before, _ := svc.q.GetArticleArchiveTotalSize(context.Background())
	if before <= archivePruneTargetBytes {
		t.Fatalf("setup size %d not above target", before)
	}
	svc.PruneArticleArchive()
	after, _ := svc.q.GetArticleArchiveTotalSize(context.Background())
	if after > before {
		t.Errorf("prune increased size?")
	}
}
