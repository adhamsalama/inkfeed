package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetPreferencesFullyPopulated(t *testing.T) {
	resetDB(t)
	uid := createTestUser(t, "full@y.com")
	ctx := userContext(uid)

	// Populate every related table.
	app.preferencesHandler(httptest.NewRecorder(), putJSON("/preferences", `{"fontSize":1.1,"letterSpacing":0.1,"lineHeight":1.2,"corsProxyUrl":"http://p","epubEmbedImages":true,"mobiEmbedImages":false,"emailTo":"e@x.com","fontFamily":"serif","boldText":true,"darkMode":false}`, uid))
	app.savedFeedsHandler(httptest.NewRecorder(), putJSON("/saved-feeds", `[{"url":"u1","title":"t1","archiveEnabled":true}]`, uid))
	app.feedGroupsHandler(httptest.NewRecorder(), putJSON("/feed-groups", `[{"name":"G","feeds":[{"url":"gu","title":"gt"},{"url":"gu2","title":"gt2"}]}]`, uid))
	app.favoritesHandler(httptest.NewRecorder(), putJSON("/favorites", `[{"url":"fu","title":"ft","feedTitle":"feed","pubDate":"pd","commentsUrl":"cu"}]`, uid))

	w := httptest.NewRecorder()
	app.preferencesHandler(w, httptest.NewRequest(http.MethodGet, "/preferences", nil).WithContext(ctx))
	if w.Code != http.StatusOK {
		t.Fatalf("GET = %d", w.Code)
	}
	var resp preferencesResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.SavedFeeds) != 1 || len(resp.FeedGroups) != 1 || len(resp.Favorites) != 1 {
		t.Errorf("populated prefs wrong: %+v", resp)
	}
	if len(resp.FeedGroups[0].Feeds) != 2 {
		t.Errorf("group items = %d", len(resp.FeedGroups[0].Feeds))
	}
	if resp.FontFamily != "serif" || !resp.BoldText || resp.MobiEmbedImages {
		t.Errorf("scalar prefs wrong: %+v", resp)
	}
}

func TestMultiArticleWithFailingURL(t *testing.T) {
	// Proxy serves a good article for /good and 500 for anything else, so the
	// second URL fails and exercises the error branches.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "good") || strings.Contains(r.URL.Path, "good") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(articleHTML))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	setProxyURL(t, srv.URL)

	combined := app.fetchAndCombine([]string{"https://example.com/good", "https://bad.invalid/x"}, "Mix")
	if !strings.Contains(combined, "Failed to fetch article") {
		t.Errorf("expected failure marker in mobi combine: %s", combined[:min(200, len(combined))])
	}

	body := app.buildEpubMultiArticleBody([]string{"https://example.com/good", "https://bad.invalid/x"}, "Mix")
	if !strings.Contains(body, "Failed to fetch article") {
		t.Errorf("expected failure marker in epub body")
	}
}

func TestEmailAndEpubSingleWithComments(t *testing.T) {
	// Article server + HN comments server (comments URL is HN).
	articleSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.RawQuery
		if strings.Contains(q, "algolia") || strings.Contains(r.URL.Path, "items") {
			w.Write([]byte(`{"id":1,"children":[{"id":2,"author":"a","text":"comment text","children":[]}]}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(articleHTML))
	})
	setProxyURL(t, articleSrv.URL)

	// epub with comments
	w := httptest.NewRecorder()
	app.epubHandler(w, postJSON("/epub", `{"url":"https://example.com/art","commentsUrl":"https://news.ycombinator.com/item?id=1","title":"WithComments","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("epub with comments = %d", w.Code)
	}

	// mobi single without TOC (no headings + no comments -> plain branch)
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{"url":"https://example.com/art","title":"Plain","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("plain mobi = %d", w.Code)
	}

	// email single with comments (epub and mobi)
	orig := brevoAPIURL
	defer func() { brevoAPIURL = orig }()
	emailSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
	brevoAPIURL = emailSrv.URL

	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://example.com/art","to":"k@k.com","commentsUrl":"https://news.ycombinator.com/item?id=1","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("email epub w/comments = %d", w.Code)
	}
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://example.com/art","to":"k@k.com","format":"mobi","commentsUrl":"https://news.ycombinator.com/item?id=1","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("email mobi w/comments = %d", w.Code)
	}
}

func TestPruneArticleArchiveLoop(t *testing.T) {
	resetDB(t)
	// Insert rows whose combined html+text length exceeds the 90MB prune target
	// so the pruning loop runs and deletes down to target.
	big := strings.Repeat("A", 20*1024*1024) // 20MB per field
	for i := 0; i < 3; i++ {
		app.archiveArticle(
			"bigkey-"+string(rune('a'+i)),
			"T", "au", "s", "2024", big, big,
		)
	}
	// 3 rows * 40MB = 120MB > 90MB target -> at least one row pruned.
	before, _ := app.q.GetArticleArchiveTotalSize(context.Background())
	if before <= archivePruneTargetBytes {
		t.Fatalf("setup size %d not above target", before)
	}
	app.pruneArticleArchive()
	after, _ := app.q.GetArticleArchiveTotalSize(context.Background())
	if after > before {
		t.Errorf("prune increased size?")
	}
}
