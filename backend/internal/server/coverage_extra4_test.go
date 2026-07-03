package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestFromGofeedPubDateFallback(t *testing.T) {
	// Atom entry whose published date gofeed cannot parse -> raw string kept.
	atom := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>T</title>
<entry><title>E</title><link href="https://example.com/a"/><summary>s</summary>
<published>not-a-real-date</published></entry></feed>`
	resp, err := parseFeed("https://x", []byte(atom))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Articles[0].PubDate != "not-a-real-date" {
		t.Errorf("gofeed pubDate fallback = %q", resp.Articles[0].PubDate)
	}
}

func TestRenderLobstersNonRFCDate(t *testing.T) {
	var sb strings.Builder
	counter := 0
	node := &lobstersNode{comment: lobstersComment{
		CreatedAt:      "2024-13-99 weird",
		Comment:        "<p>c</p>",
		CommentingUser: json.RawMessage(`"user"`),
	}}
	renderLobstersComment(&sb, node, &counter, true)
	if !strings.Contains(sb.String(), "2024-13-99") {
		t.Errorf("date-prefix fallback missing: %s", sb.String())
	}
}

func TestDecodeViaAPIParseBranches(t *testing.T) {
	orig := googleNewsBatchExecuteURL
	defer func() { googleNewsBatchExecuteURL = orig }()

	cases := []string{
		")]}'\n\nnot-json-here",                                 // outer unmarshal fails
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\"]]",                     // row has <3 elements
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",123]]",                 // row[2] not a string
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",\"[]\"]]",              // inner array too short
		")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",\"[\\\"x\\\",123]\"]]", // inner[1] not a string
	}
	for i, resp := range cases {
		body := resp
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		})
		googleNewsBatchExecuteURL = srv.URL
		if _, err := decodeViaAPI("b64", "sig", "ts"); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestFromRSSPubDateFallback(t *testing.T) {
	// pubDate that gofeed cannot parse falls back to the raw string; the entry
	// has no comments and is not a Reddit link.
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
