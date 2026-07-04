package export

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	readability "github.com/go-shiori/go-readability"
)

// fakeFetcher implements Fetcher without any network: it returns a canned
// article for each URL (unless listed in fail) and canned comments.
type fakeFetcher struct {
	fail     map[string]bool
	comments string
	content  string // overrides the default article body when set
}

func (f fakeFetcher) FetchReadable(rawURL string) (readability.Article, error) {
	if f.fail[rawURL] {
		return readability.Article{}, fmt.Errorf("fetch failed: %s", rawURL)
	}
	body := f.content
	if body == "" {
		body = "<h2>Section</h2><p>body content</p>"
	}
	return readability.Article{
		Title:       "Title " + rawURL,
		Content:     body,
		TextContent: "body content",
	}, nil
}

func (f fakeFetcher) FetchComments(rawURL string) string { return f.comments }

func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}
