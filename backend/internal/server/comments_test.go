package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripHTMLTags(t *testing.T) {
	if got := stripHTMLTags("<p>Hello <b>world</b></p>"); got != "Hello world" {
		t.Errorf("stripHTMLTags = %q", got)
	}
	if got := stripHTMLTags("plain"); got != "plain" {
		t.Errorf("plain = %q", got)
	}
}

func TestLobstersUsername(t *testing.T) {
	if got := lobstersUsername(json.RawMessage(`{"username":"alice"}`)); got != "alice" {
		t.Errorf("object form = %q", got)
	}
	if got := lobstersUsername(json.RawMessage(`"bob"`)); got != "bob" {
		t.Errorf("string form = %q", got)
	}
	if got := lobstersUsername(json.RawMessage(``)); got != "[deleted]" {
		t.Errorf("empty = %q", got)
	}
	if got := lobstersUsername(json.RawMessage(`123`)); got != "[deleted]" {
		t.Errorf("number = %q", got)
	}
}

func TestLobstersJSONURL(t *testing.T) {
	got, err := lobstersJSONURL("https://lobste.rs/s/abc123/some-title")
	if err != nil || got != "https://lobste.rs/s/abc123.json" {
		t.Errorf("got %q err %v", got, err)
	}
	got, _ = lobstersJSONURL("https://lobste.rs/s/xyz")
	if got != "https://lobste.rs/s/xyz.json" {
		t.Errorf("no-title = %q", got)
	}
	if _, err := lobstersJSONURL("https://lobste.rs/nope"); err == nil {
		t.Error("expected error without /s/")
	}
	if _, err := lobstersJSONURL("https://lobste.rs/s/"); err == nil {
		t.Error("expected error empty short id")
	}
}

func TestBuildLobstersTree(t *testing.T) {
	comments := []lobstersComment{
		{ShortID: "a", ParentComment: ""},
		{ShortID: "b", ParentComment: "a"},
		{ShortID: "c", ParentComment: "missing"}, // orphan -> root
	}
	roots := buildLobstersTree(comments)
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2", len(roots))
	}
	// find node a and check it has child b
	var a *lobstersNode
	for _, r := range roots {
		if r.comment.ShortID == "a" {
			a = r
		}
	}
	if a == nil || len(a.children) != 1 || a.children[0].comment.ShortID != "b" {
		t.Errorf("tree structure wrong: %+v", a)
	}
}

func TestRenderHNComment(t *testing.T) {
	var sb strings.Builder
	counter := 0
	item := hnItem{
		Author:    "hnuser",
		CreatedAt: "2024-01-02T03:04:05.000Z",
		Text:      "<p>a comment</p>",
		Children: []hnItem{
			{Author: "", Text: ""}, // deleted child
		},
	}
	renderHNComment(&sb, item, 0, &counter, false)
	out := sb.String()
	if !strings.Contains(out, "hnuser") || !strings.Contains(out, "a comment") {
		t.Errorf("HN render missing content: %s", out)
	}
	if !strings.Contains(out, "[deleted]") {
		t.Errorf("deleted child not rendered")
	}
	if !strings.Contains(out, "2024-01-02") {
		t.Errorf("date not rendered")
	}

	// collapsed variant
	sb.Reset()
	counter = 0
	renderHNComment(&sb, hnItem{Author: "u", Text: "x", CreatedAt: "bad-date-string"}, 0, &counter, true)
	if !strings.Contains(sb.String(), "[+]") {
		t.Errorf("collapsed toggle missing")
	}
}

func TestRenderRedditComment(t *testing.T) {
	var sb strings.Builder
	counter := 0
	replyCount := 0
	// "more" kind is skipped
	renderRedditComment(&sb, redditThing{Kind: "more"}, 0, true, &replyCount, &counter, false)
	if sb.Len() != 0 {
		t.Errorf("more kind should render nothing")
	}

	sb.Reset()
	counter = 0
	replyCount = 0
	replies := `{"kind":"Listing","data":{"children":[{"kind":"t1","data":{"author":"child","body_html":"&lt;p&gt;reply&lt;/p&gt;","created_utc":1700000000}}]}}`
	thing := redditThing{
		Kind: "t1",
		Data: redditComment{
			Author:     "op",
			BodyHTML:   "&lt;p&gt;hello&lt;/p&gt;",
			CreatedUTC: 1700000000,
			Replies:    json.RawMessage(replies),
		},
	}
	renderRedditComment(&sb, thing, 0, true, &replyCount, &counter, false)
	out := sb.String()
	if !strings.Contains(out, "op") || !strings.Contains(out, "hello") {
		t.Errorf("reddit render missing: %s", out)
	}
	if !strings.Contains(out, "child") || !strings.Contains(out, "reply") {
		t.Errorf("reddit reply not rendered: %s", out)
	}
}

func TestRenderLobstersComment(t *testing.T) {
	var sb strings.Builder
	counter := 0
	node := &lobstersNode{
		comment: lobstersComment{
			CreatedAt:      "2024-05-06T00:00:00.000Z",
			Comment:        "<p>lobster comment</p>",
			CommentingUser: json.RawMessage(`{"username":"lobster"}`),
		},
		children: []*lobstersNode{
			{comment: lobstersComment{IsDeleted: true}},
		},
	}
	renderLobstersComment(&sb, node, &counter, false)
	out := sb.String()
	if !strings.Contains(out, "lobster") || !strings.Contains(out, "lobster comment") {
		t.Errorf("lobsters render missing: %s", out)
	}
	if !strings.Contains(out, "[deleted]") {
		t.Errorf("deleted child not rendered")
	}
}

// ── network-backed fetchers via proxy ──

func TestFetchHNComments(t *testing.T) {
	hnJSON := `{"id":1,"children":[{"id":2,"author":"a","created_at":"2024-01-01T00:00:00Z","text":"hi","children":[]}]}`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(hnJSON))
	})
	setProxyURL(t, srv.URL)
	out, err := app.fetchHNComments("https://news.ycombinator.com/item?id=12345")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("HN comments = %s", out)
	}

	// No item id
	if _, err := app.fetchHNComments("https://news.ycombinator.com/item"); err == nil {
		t.Error("expected error without id")
	}
	// empty id
	if _, err := app.fetchHNComments("https://news.ycombinator.com/item?id="); err == nil {
		t.Error("expected error empty id")
	}

	// no comments
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"children":[]}`))
	})
	setProxyURL(t, srv2.URL)
	out, _ = app.fetchHNComments("https://news.ycombinator.com/item?id=1")
	if !strings.Contains(out, "No comments") {
		t.Errorf("expected no comments msg: %s", out)
	}
}

func TestFetchRedditComments(t *testing.T) {
	redditJSON := `[{"data":{"children":[]}},{"data":{"children":[{"kind":"t1","data":{"author":"u","body_html":"&lt;p&gt;c&lt;/p&gt;","created_utc":1700000000}}]}}]`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(redditJSON))
	})
	setProxyURL(t, srv.URL)
	out, err := app.fetchRedditComments("https://reddit.com/r/x/comments/y/.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "u") {
		t.Errorf("reddit comments = %s", out)
	}

	// empty comments
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"data":{"children":[]}},{"data":{"children":[]}}]`))
	})
	setProxyURL(t, srv2.URL)
	out, _ = app.fetchRedditComments("https://reddit.com/x/.json")
	if !strings.Contains(out, "No comments") {
		t.Errorf("expected no comments: %s", out)
	}
}

func TestFetchLobsteComments(t *testing.T) {
	lobJSON := `{"comments":[{"short_id":"a","comment":"<p>hey</p>","commenting_user":{"username":"lob"},"created_at":"2024-01-01T00:00:00.000Z"}]}`
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(lobJSON))
	})
	setProxyURL(t, srv.URL)
	out, err := app.fetchLobsteComments("https://lobste.rs/s/abc/title")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "lob") {
		t.Errorf("lobsters = %s", out)
	}

	// bad url
	if _, err := app.fetchLobsteComments("https://lobste.rs/nope"); err == nil {
		t.Error("expected error")
	}
}

func TestFetchCommentsHTML(t *testing.T) {
	if app.fetchCommentsHTML("") != "" {
		t.Error("empty url -> empty")
	}

	// HN routing
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"children":[{"id":2,"author":"a","text":"hi","children":[]}]}`))
	})
	setProxyURL(t, srv.URL)
	if !strings.Contains(app.fetchCommentsHTML("https://news.ycombinator.com/item?id=1"), "hi") {
		t.Error("HN routing failed")
	}

	// Fallback to readability for generic URL
	serveArticleViaProxy(t, articleHTML)
	if app.fetchCommentsHTML("https://example.com/generic") == "" {
		t.Error("generic fallback returned empty")
	}

	// Error case returns empty (not error): generic URL unreachable via both
	// proxy and direct.
	setProxyURL(t, "http://127.0.0.1:0/dead")
	if app.fetchCommentsHTML("http://127.0.0.1:0/dead-generic") != "" {
		t.Error("fetch error should return empty string")
	}
}

func TestCommentsHandler(t *testing.T) {
	// missing url
	w := httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url = %d", w.Code)
	}

	// HN
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":1,"children":[{"id":2,"author":"a","text":"hi","children":[]}]}`))
	})
	setProxyURL(t, srv.URL)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://news.ycombinator.com/item?id=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HN comments handler = %d", w.Code)
	}
	var resp CommentsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.HTML, "hi") {
		t.Errorf("HN handler html = %s", resp.HTML)
	}

	// Reddit
	srvR := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"data":{"children":[]}},{"data":{"children":[{"kind":"t1","data":{"author":"u","body_html":"c","created_utc":1}}]}}]`))
	})
	setProxyURL(t, srvR.URL)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://reddit.com/x/.json", nil))
	if w.Code != http.StatusOK {
		t.Errorf("reddit handler = %d", w.Code)
	}

	// Lobsters
	srvL := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"comments":[{"short_id":"a","comment":"hi","commenting_user":"lob","created_at":"2024-01-01T00:00:00Z"}]}`))
	})
	setProxyURL(t, srvL.URL)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://lobste.rs/s/abc/t", nil))
	if w.Code != http.StatusOK {
		t.Errorf("lobsters handler = %d", w.Code)
	}

	// Generic readability
	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=https://example.com/g", nil))
	if w.Code != http.StatusOK {
		t.Errorf("generic handler = %d", w.Code)
	}

	// Generic error
	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.commentsHandler(w, httptest.NewRequest(http.MethodGet, "/comments?url=http://127.0.0.1:0/dead", nil))
	if w.Code != http.StatusBadGateway {
		t.Errorf("generic error = %d", w.Code)
	}
}
