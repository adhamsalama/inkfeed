package server

import (
	"net/http"
	"testing"
)

// articleHTML is a page rich enough for Readability to extract an article body.
// Used by the export/email handler tests that fetch through the content proxy.
const articleHTML = `<!DOCTYPE html><html><head><title>Test Article Title</title>
<meta name="author" content="Jane Author"></head><body>
<article><h1>Test Article Title</h1>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
</article></body></html>`

const longParagraph = "This is a reasonably long paragraph of article content that Readability should extract as the main body text because it contains enough words to be considered meaningful content rather than boilerplate navigation or advertising material that surrounds it."

// serveArticleViaProxy points the content service's proxy at a server returning
// html, so handler tests exercise the fetch path without real network access.
func serveArticleViaProxy(t *testing.T, html string) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})
	setProxyURL(t, srv.URL)
}
