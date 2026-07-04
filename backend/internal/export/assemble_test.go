package export

import (
	"net/http"
	"strings"
	"testing"
)

// These tests assert on the assembled MOBI HTML (not just that Render returned
// non-empty bytes), so regressions in comment inclusion, image embedding, and
// TOC filepos resolution are actually caught.

func TestAssembleMobiIncludesComments(t *testing.T) {
	html, _, _, err := assembleMobi(
		fakeFetcher{comments: "<p>a reader comment</p>"},
		Request{URL: "https://example.com/a", CommentsURL: "https://news.ycombinator.com/item?id=1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "a reader comment") || !strings.Contains(html, "<h2>Comments</h2>") {
		t.Errorf("single-article MOBI dropped comments:\n%s", html)
	}
}

func TestAssembleMobiEmbedsImages(t *testing.T) {
	imgSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(makePNG(t, false))
	})
	html, images, _, err := assembleMobi(
		fakeFetcher{content: `<p>x</p><img src="` + imgSrv.URL + `/pic.png">`},
		Request{URL: "https://example.com/a", EmbedImages: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 embedded image record, got %d", len(images))
	}
	if !strings.Contains(html, `recindex="1"`) {
		t.Errorf("image not rewritten to recindex:\n%s", html)
	}
}

func TestAssembleMobiResolvesFilepos(t *testing.T) {
	// Two headings + comments -> the single-article TOC branch emits filepos
	// placeholders that must be patched to real offsets.
	html, _, _, err := assembleMobi(
		fakeFetcher{content: "<h2>One</h2><p>a</p><h2>Two</h2><p>b</p>", comments: "<p>c</p>"},
		Request{URL: "https://example.com/a", CommentsURL: "https://news.ycombinator.com/item?id=1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `filepos="`) {
		t.Fatalf("expected a table of contents with filepos links:\n%s", html)
	}
	if strings.Contains(html, mobiTOCPlaceholder) {
		t.Errorf("TOC filepos placeholders were never resolved:\n%s", html)
	}
}
