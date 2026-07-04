package content

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStartBackgroundJobs(t *testing.T) {
	resetDB(t)
	// These spawn goroutines that operate on the empty test DB (no saved feeds),
	// so they are effectively no-ops; we only assert they launch without panic.
	svc.StartFeedScraper()
	svc.StartContentArchiver()
	svc.StartFeedItemsPruner()
	svc.StartArticleArchivePruner()
}

func TestFromGofeedPubDateAndComments(t *testing.T) {
	// Atom entry linking to Reddit -> comments get /.json; unparseable published
	// falls back to the raw string.
	atom := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>T</title>
<entry><title>E</title><link href="https://reddit.com/r/x/comments/abc"/>
<summary>s</summary><published>not-a-date</published></entry></feed>`
	resp, err := parseFeed("https://x", []byte(atom))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Articles[0].Comments != "https://reddit.com/r/x/comments/abc/.json" {
		t.Errorf("comments = %q", resp.Articles[0].Comments)
	}
	if resp.Articles[0].PubDate != "not-a-date" {
		t.Errorf("pubDate fallback = %q", resp.Articles[0].PubDate)
	}
}

func TestFromRSSPubDateFallback(t *testing.T) {
	rss := `<?xml version="1.0"?><rss version="2.0"><channel><title>T</title>
<item><title>I</title><link>https://example.com/x</link><description>d</description>
<pubDate>totally not a date</pubDate></item></channel></rss>`
	resp, err := parseFeed("https://x", []byte(rss))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Articles[0].PubDate != "totally not a date" {
		t.Errorf("pubDate fallback = %q", resp.Articles[0].PubDate)
	}
}

func TestDecodeViaAPIParseBranches(t *testing.T) {
	orig := googleNewsBatchExecuteURL
	defer func() { googleNewsBatchExecuteURL = orig }()

	cases := []string{
		")]}'\n\nnot-json-here",
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\"]]",
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",123]]",
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",\"[]\"]]",
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",\"[\\\"x\\\",123]\"]]",
	}
	for i, resp := range cases {
		body := resp
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
		googleNewsBatchExecuteURL = srv.URL
		if _, err := decodeViaAPI("b64", "sig", "ts"); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestDecodeGoogleNewsURLFlow(t *testing.T) {
	articleSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<c-wiz><div jscontroller="c" data-n-a-sg="SIG" data-n-a-ts="TS">x</div></c-wiz>`))
	})
	batchSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		inner, _ := json.Marshal([]string{"garturlres", "https://decoded.example/final"})
		row, _ := json.Marshal([]any{"wrb.fr", "Fbv4je", string(inner)})
		outer, _ := json.Marshal([]json.RawMessage{row})
		w.Write([]byte(")]}'\n\n" + string(outer)))
	})
	origA, origB := googleNewsArticleURLs, googleNewsBatchExecuteURL
	googleNewsArticleURLs = []string{articleSrv.URL + "/articles/%s"}
	googleNewsBatchExecuteURL = batchSrv.URL
	defer func() { googleNewsArticleURLs, googleNewsBatchExecuteURL = origA, origB }()

	got, err := DecodeGoogleNewsURL("https://news.google.com/articles/CBMiABC")
	if err != nil || got != "https://decoded.example/final" {
		t.Fatalf("decoded = %q err=%v", got, err)
	}
	if _, err := DecodeGoogleNewsURL("https://example.com/x"); err == nil {
		t.Error("expected error for non-google url")
	}
	_ = strings.TrimSpace("")
}
