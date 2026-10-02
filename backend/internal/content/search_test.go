package content

import (
	"context"
	"strings"
	"testing"

	"github.com/adhamsalama/inkfeed-backend/db"
)

func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"":                       "",
		"   ":                    "",
		"go":                     `"go"`,
		"  rust   go ":           `"rust" "go"`,
		`say "hi" NOT foo* (x):`: `"say" "hi" "NOT" "foo*" "(x):"`,
		`""`:                     "",
	}
	for in, want := range cases {
		if got := FTSQuery(in); got != want {
			t.Errorf("FTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchSnippet(t *testing.T) {
	if got := matchSnippet("nothing here", []string{"absent"}); got != "" {
		t.Errorf("no match = %q", got)
	}
	if got := matchSnippet("Hello  World\n\nagain", []string{"world"}); got != "Hello World again" {
		t.Errorf("short text = %q", got)
	}

	long := strings.Repeat("filler ", 40) + "the Searches keyword appears " + strings.Repeat("tail ", 60)
	got := matchSnippet(long, []string{"search"})
	if !strings.HasPrefix(got, "…filler") || !strings.HasSuffix(got, "…") {
		t.Errorf("long snippet boundaries = %q", got)
	}
	if !strings.Contains(got, "Searches keyword") {
		t.Errorf("snippet missing match: %q", got)
	}
	// Stemmed form not present verbatim falls back to a trimmed prefix.
	if got := matchSnippet("we were searching widely", []string{"searches"}); !strings.Contains(got, "searching") {
		t.Errorf("stem fallback = %q", got)
	}
}

func TestSearchFeedArchive(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	seed := func(feed, url, title, text string) {
		if _, err := svc.q.InsertFeedItem(ctx, db.InsertFeedItemParams{
			FeedUrl: feed, ItemUrl: url, Title: title, Description: "feed desc", PubDate: "2024-01-01T00:00:00Z",
		}); err != nil {
			t.Fatal(err)
		}
		svc.ArchiveArticle(url, title, "", "", "", "<p>"+text+"</p>", text)
	}
	seed("feedA", "https://a/1", "Gophers", "Concurrency in Go uses goroutines and channels.")
	seed("feedA", "https://a/2", "Crabs", "Rust has ownership & <borrowing>.")
	seed("feedB", "https://b/1", "Other goroutines", "Goroutines elsewhere.")

	page, err := svc.SearchFeedArchive("feedA", "goroutine", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Articles) != 1 || page.Articles[0].Link != "https://a/1" {
		t.Fatalf("stemmed search = %+v", page)
	}
	if !strings.Contains(page.Articles[0].Description, "goroutines and channels") {
		t.Errorf("snippet = %q", page.Articles[0].Description)
	}

	// Title is indexed too; snippet falls back to the feed description.
	page, _ = svc.SearchFeedArchive("feedA", "crabs", 10, 0)
	if len(page.Articles) != 1 || page.Articles[0].Description == "" {
		t.Errorf("title search = %+v", page)
	}

	// Snippets are HTML-escaped.
	page, _ = svc.SearchFeedArchive("feedA", "borrowing", 10, 0)
	if len(page.Articles) != 1 || !strings.Contains(page.Articles[0].Description, "&lt;borrowing&gt;") {
		t.Errorf("escaped snippet = %+v", page)
	}

	// All terms must match.
	page, _ = svc.SearchFeedArchive("feedA", "rust goroutines", 10, 0)
	if len(page.Articles) != 0 {
		t.Errorf("AND search = %+v", page)
	}

	// FTS5 syntax in user input is harmless.
	if _, err := svc.SearchFeedArchive("feedA", `"unbalanced ( NOT * :`, 10, 0); err != nil {
		t.Errorf("operator input errored: %v", err)
	}
	page, err = svc.SearchFeedArchive("feedA", `"" `, 10, 0)
	if err != nil || len(page.Articles) != 0 || page.Articles == nil {
		t.Errorf("empty query = %+v, %v", page, err)
	}

	// Re-archiving replaces the indexed text; deleting removes it.
	svc.ArchiveArticle("https://a/1", "Gophers", "", "", "", "", "Now about generics only.")
	if page, _ = svc.SearchFeedArchive("feedA", "channels", 10, 0); len(page.Articles) != 0 {
		t.Errorf("stale index after update = %+v", page)
	}
	if page, _ = svc.SearchFeedArchive("feedA", "generics", 10, 0); len(page.Articles) != 1 {
		t.Errorf("updated text not indexed = %+v", page)
	}
	if _, err := testDB.Exec(`DELETE FROM article_archive WHERE key = 'https://a/1'`); err != nil {
		t.Fatal(err)
	}
	if page, _ = svc.SearchFeedArchive("feedA", "generics", 10, 0); len(page.Articles) != 0 {
		t.Errorf("stale index after delete = %+v", page)
	}
}

func TestSearchFeedArchivePagination(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	for _, u := range []string{"https://p/1", "https://p/2", "https://p/3"} {
		svc.q.InsertFeedItem(ctx, db.InsertFeedItemParams{FeedUrl: "feedP", ItemUrl: u, Title: "kindle " + u})
		svc.ArchiveArticle(u, "kindle "+u, "", "", "", "", "e-ink reading")
	}
	page, err := svc.SearchFeedArchive("feedP", "kindle", 2, 0)
	if err != nil || page.Total != 3 || len(page.Articles) != 2 || !page.HasMore {
		t.Fatalf("page 1 = %+v, %v", page, err)
	}
	page, _ = svc.SearchFeedArchive("feedP", "kindle", 2, 2)
	if len(page.Articles) != 1 || page.HasMore || page.Articles[0].Index != 2 {
		t.Errorf("page 2 = %+v", page)
	}
}
