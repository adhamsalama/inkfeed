package content

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	readability "github.com/go-shiori/go-readability"
)

// articleHTML is a page rich enough for Readability to extract an article body.
const articleHTML = `<!DOCTYPE html><html><head><title>Test Article Title</title>
<meta name="author" content="Jane Author"></head><body>
<article><h1>Test Article Title</h1>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
</article></body></html>`

const longParagraph = "This is a reasonably long paragraph of article content that Readability should extract as the main body text because it contains enough words to be considered meaningful content rather than boilerplate navigation or advertising material that surrounds it."

// serveArticleViaProxy points the scraping proxy at a server returning html.
func serveArticleViaProxy(t *testing.T, html string) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})
	setProxyURL(t, srv.URL)
}

func TestArticleMetaHTML(t *testing.T) {
	pub := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	a := readability.Article{
		Byline:        "Jane",
		SiteName:      "Example",
		TextContent:   strings.Repeat("word ", 400),
		PublishedTime: &pub,
	}
	got := ArticleMeta(a)
	if !strings.Contains(got, "Jane @ Example") {
		t.Errorf("byline line missing: %q", got)
	}
	if !strings.Contains(got, "min read") {
		t.Errorf("reading time missing: %q", got)
	}
	if !strings.Contains(got, "Published:") {
		t.Errorf("published line missing")
	}
	if !strings.Contains(got, "<hr/>") {
		t.Errorf("trailing hr missing")
	}

	// Only byline
	got = ArticleMeta(readability.Article{Byline: "Solo"})
	if !strings.Contains(got, "Solo") || strings.Contains(got, "@") {
		t.Errorf("byline-only = %q", got)
	}
	// Only site name
	got = ArticleMeta(readability.Article{SiteName: "SiteOnly"})
	if !strings.Contains(got, "SiteOnly") {
		t.Errorf("site-only = %q", got)
	}
	// Empty article -> empty output
	if ArticleMeta(readability.Article{}) != "" {
		t.Errorf("empty article should give empty meta")
	}
	// Short text -> min 1 read
	got = ArticleMeta(readability.Article{TextContent: "few words here"})
	if !strings.Contains(got, "1 min read") {
		t.Errorf("expected 1 min read, got %q", got)
	}
}

func TestDedupeResponsiveImages(t *testing.T) {
	html := `<p>x</p>` +
		`<img src="https://cdn.example/photo-desktop.jpg" alt="a">` +
		`<img src="https://cdn.example/photo-mobile.jpg" alt="a">` +
		`<img src="https://cdn.example/other.jpg">`
	out := dedupeResponsiveImages(html)
	// The desktop/mobile pair collapses to one; the unrelated image remains.
	if strings.Count(out, "<img") != 2 {
		t.Errorf("expected 2 imgs after dedupe, got %d: %s", strings.Count(out, "<img"), out)
	}
	if !strings.Contains(out, "other.jpg") {
		t.Errorf("non-responsive image dropped")
	}

	// No src -> unchanged
	noSrc := `<img alt="x">`
	if dedupeResponsiveImages(noSrc) != noSrc {
		t.Errorf("img without src should be unchanged")
	}
}

func TestFetchReadable(t *testing.T) {
	serveArticleViaProxy(t, articleHTML)
	a, err := svc.FetchReadable("https://example.com/post")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Title, "Test Article Title") {
		t.Errorf("title = %q", a.Title)
	}
	if !strings.Contains(a.TextContent, "long paragraph") {
		t.Errorf("content not extracted: %q", a.TextContent)
	}
}

func TestFetchReadableError(t *testing.T) {
	setProxyURL(t, "http://127.0.0.1:0/dead")
	if _, err := svc.FetchReadable("http://127.0.0.1:0/dead"); err == nil {
		t.Error("expected error")
	}
}

func TestArchiveAndPrune(t *testing.T) {
	resetDB(t)
	// Nothing to prune when under target.
	svc.PruneArticleArchive()

	svc.ArchiveArticle("k1", "T", "A", "S", "2024", "<p>hi</p>", "hi there")
	row, err := svc.q.GetArticleArchive(context.Background(), "k1")
	if err != nil || row.Title != "T" {
		t.Fatalf("archive not written: %v", err)
	}
}
