package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adhamsalama/inkfeed-backend/db"
	_ "modernc.org/sqlite"
)

// withBrokenDB points the global queries at a closed database connection so that
// every query fails, then restores the real queries afterwards. This exercises
// the "internal error" branches in handlers cheaply.
func withBrokenDB(t *testing.T) func() {
	t.Helper()
	bad, err := sql.Open("sqlite", "file:broken?mode=memory")
	if err != nil {
		t.Fatal(err)
	}
	bad.Close() // closed -> all queries error
	prev := app.q
	app.q = db.New(bad)
	return func() { app.q = prev }
}

func TestHandlersWithBrokenDB(t *testing.T) {
	uid := int64(1)

	tests := []struct {
		name    string
		req     func() *http.Request
		handler http.HandlerFunc
		want    int
	}{
		{
			"signup",
			func() *http.Request { return postJSON("/signup", `{"email":"b@y.com","password":"Abcdefgh1!"}`) },
			app.signupHandler, http.StatusConflict, // CreateUser fails -> treated as conflict
		},
		{
			"signin",
			func() *http.Request { return postJSON("/signin", `{"email":"b@y.com","password":"Abcdefgh1!"}`) },
			app.signinHandler, http.StatusInternalServerError,
		},
		{
			"changePassword",
			func() *http.Request {
				return postJSON("/change-password", `{"currentPassword":"Abcdefgh1!","newPassword":"NewPass123!","confirmPassword":"NewPass123!"}`).WithContext(userContext(uid))
			},
			app.changePasswordHandler, http.StatusInternalServerError,
		},
		{
			"getPreferences",
			func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/preferences", nil).WithContext(userContext(uid))
			},
			app.preferencesHandler, http.StatusInternalServerError,
		},
		{
			"putPreferences",
			func() *http.Request { return putJSON("/preferences", `{"fontSize":1}`, uid) },
			app.preferencesHandler, http.StatusInternalServerError,
		},
		{
			"savedFeeds",
			func() *http.Request { return putJSON("/saved-feeds", `[{"url":"u","title":"t"}]`, uid) },
			app.savedFeedsHandler, http.StatusInternalServerError,
		},
		{
			"feedGroups",
			func() *http.Request { return putJSON("/feed-groups", `[{"name":"g","feeds":[]}]`, uid) },
			app.feedGroupsHandler, http.StatusInternalServerError,
		},
		{
			"favorites",
			func() *http.Request { return putJSON("/favorites", `[{"url":"u","title":"t"}]`, uid) },
			app.favoritesHandler, http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			restore := withBrokenDB(t)
			defer restore()
			w := httptest.NewRecorder()
			tc.handler(w, tc.req())
			if w.Code != tc.want {
				t.Errorf("%s: code = %d, want %d", tc.name, w.Code, tc.want)
			}
		})
	}
}
