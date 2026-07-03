package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMobiHandlerWithTOCAndComments(t *testing.T) {
	// Article with multiple headings + a comments URL exercises the single-article
	// TOC branch end to end through the /mobi handler.
	articleSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(articleWithHeadings))
	})
	setProxyURL(t, articleSrv.URL)

	body := `{"url":"https://example.com/big","title":"Big","commentsUrl":"https://news.ycombinator.com/item?id=1","embedImages":false}`
	w := httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", body))
	if w.Code != http.StatusOK {
		t.Fatalf("mobi with toc = %d body=%s", w.Code, w.Body.String())
	}
}

const articleWithHeadings = `<!DOCTYPE html><html><head><title>Big Article</title></head><body>
<article><h1>Big Article</h1>
<h2>Section One</h2><p>` + longParagraph + `</p>
<h2>Section Two</h2><p>` + longParagraph + `</p>
<p>` + longParagraph + `</p>
</article></body></html>`
