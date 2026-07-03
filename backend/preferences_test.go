package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func putJSON(path, body string, userID int64) *http.Request {
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(userContext(userID))
}

func TestMaxArchivedFeeds(t *testing.T) {
	os.Unsetenv("MAX_ARCHIVED_FEEDS")
	if maxArchivedFeeds() != 50 {
		t.Errorf("default = %d", maxArchivedFeeds())
	}
	os.Setenv("MAX_ARCHIVED_FEEDS", "10")
	defer os.Unsetenv("MAX_ARCHIVED_FEEDS")
	if maxArchivedFeeds() != 10 {
		t.Errorf("env = %d", maxArchivedFeeds())
	}
	os.Setenv("MAX_ARCHIVED_FEEDS", "notanumber")
	if maxArchivedFeeds() != 50 {
		t.Errorf("invalid env = %d", maxArchivedFeeds())
	}
}

func TestPreferencesHandlerRoundTrip(t *testing.T) {
	resetDB(t)
	uid := createTestUser(t, "prefs@y.com")

	// GET with no preferences yet -> defaults (embed images true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/preferences", nil).WithContext(userContext(uid))
	app.preferencesHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET prefs = %d", w.Code)
	}
	var resp preferencesResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Email != "prefs@y.com" || !resp.EpubEmbedImages || !resp.MobiEmbedImages {
		t.Errorf("default prefs wrong: %+v", resp)
	}

	// PUT preferences
	body := `{"fontSize":1.5,"letterSpacing":0.2,"lineHeight":1.6,"corsProxyUrl":"http://p","epubEmbedImages":false,"mobiEmbedImages":true,"emailTo":"k@x.com","fontFamily":"serif","boldText":true,"darkMode":true}`
	w = httptest.NewRecorder()
	app.preferencesHandler(w, putJSON("/preferences", body, uid))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT prefs = %d", w.Code)
	}

	// GET reflects saved values
	w = httptest.NewRecorder()
	app.preferencesHandler(w, httptest.NewRequest(http.MethodGet, "/preferences", nil).WithContext(userContext(uid)))
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.FontSize != 1.5 || resp.EmailTo != "k@x.com" || resp.EpubEmbedImages || !resp.DarkMode {
		t.Errorf("saved prefs wrong: %+v", resp)
	}

	// PUT bad JSON
	w = httptest.NewRecorder()
	app.preferencesHandler(w, putJSON("/preferences", `{bad`, uid))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad PUT = %d", w.Code)
	}

	// Unsupported method
	w = httptest.NewRecorder()
	app.preferencesHandler(w, httptest.NewRequest(http.MethodDelete, "/preferences", nil).WithContext(userContext(uid)))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE = %d", w.Code)
	}
}

func TestSavedFeedsHandler(t *testing.T) {
	resetDB(t)
	uid := createTestUser(t, "sf@y.com")

	// wrong method
	w := httptest.NewRecorder()
	app.savedFeedsHandler(w, httptest.NewRequest(http.MethodGet, "/saved-feeds", nil).WithContext(userContext(uid)))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	// bad json
	w = httptest.NewRecorder()
	app.savedFeedsHandler(w, putJSON("/saved-feeds", `{bad`, uid))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d", w.Code)
	}

	// valid
	w = httptest.NewRecorder()
	app.savedFeedsHandler(w, putJSON("/saved-feeds", `[{"url":"u1","title":"T1","archiveEnabled":true},{"url":"u2","title":"T2"}]`, uid))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d", w.Code)
	}
	feeds, _ := app.q.GetUserSavedFeeds(userContext(uid), uid)
	if len(feeds) != 2 {
		t.Errorf("saved feeds = %d", len(feeds))
	}

	// exceeds archive limit
	os.Setenv("MAX_ARCHIVED_FEEDS", "1")
	defer os.Unsetenv("MAX_ARCHIVED_FEEDS")
	w = httptest.NewRecorder()
	app.savedFeedsHandler(w, putJSON("/saved-feeds", `[{"url":"a","title":"A","archiveEnabled":true},{"url":"b","title":"B","archiveEnabled":true}]`, uid))
	if w.Code != http.StatusBadRequest {
		t.Errorf("over limit = %d", w.Code)
	}
}

func TestFeedGroupsHandler(t *testing.T) {
	resetDB(t)
	uid := createTestUser(t, "fg@y.com")

	w := httptest.NewRecorder()
	app.feedGroupsHandler(w, httptest.NewRequest(http.MethodGet, "/feed-groups", nil).WithContext(userContext(uid)))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	w = httptest.NewRecorder()
	app.feedGroupsHandler(w, putJSON("/feed-groups", `{bad`, uid))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d", w.Code)
	}

	w = httptest.NewRecorder()
	app.feedGroupsHandler(w, putJSON("/feed-groups", `[{"name":"G1","feeds":[{"url":"u","title":"t"}]}]`, uid))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d", w.Code)
	}
	groups, _ := app.q.GetUserFeedGroups(userContext(uid), uid)
	if len(groups) != 1 {
		t.Errorf("groups = %d", len(groups))
	}
}

func TestFavoritesHandler(t *testing.T) {
	resetDB(t)
	uid := createTestUser(t, "favs@y.com")

	w := httptest.NewRecorder()
	app.favoritesHandler(w, httptest.NewRequest(http.MethodGet, "/favorites", nil).WithContext(userContext(uid)))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	w = httptest.NewRecorder()
	app.favoritesHandler(w, putJSON("/favorites", `{bad`, uid))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d", w.Code)
	}

	w = httptest.NewRecorder()
	app.favoritesHandler(w, putJSON("/favorites", `[{"url":"u","title":"t","feedTitle":"ft","pubDate":"pd","commentsUrl":"cu"}]`, uid))
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d", w.Code)
	}
	favs, _ := app.q.GetUserFavorites(userContext(uid), uid)
	if len(favs) != 1 {
		t.Errorf("favs = %d", len(favs))
	}
}

func TestSignoutHandler(t *testing.T) {
	resetDB(t)

	// wrong method
	w := httptest.NewRecorder()
	app.signoutHandler(w, httptest.NewRequest(http.MethodGet, "/signout", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	// no cookie -> still clears
	w = httptest.NewRecorder()
	app.signoutHandler(w, httptest.NewRequest(http.MethodPost, "/signout", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("no cookie signout = %d", w.Code)
	}

	// with cookie
	uid := createTestUser(t, "so@y.com")
	iw := httptest.NewRecorder()
	app.issueSession(iw, httptest.NewRequest(http.MethodPost, "/signout", nil), uid)
	cookie := iw.Result().Cookies()[0]
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signout", nil)
	req.AddCookie(cookie)
	app.signoutHandler(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("signout = %d", w.Code)
	}
	if _, err := app.q.GetSession(userContext(uid), cookie.Value); err == nil {
		t.Errorf("session should be deleted")
	}
}
