package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		pwd     string
		wantErr bool
	}{
		{"Abcdefgh1!", false},
		{"short1!", true},      // too short
		{"abcdefghij!", true},  // no digit
		{"abcdefghij1", true},  // no symbol
		{"1234567890!", false}, // digits + symbol, len ok
		{"", true},
	}
	for _, c := range cases {
		err := validatePassword(c.pwd)
		if (err != nil) != c.wantErr {
			t.Errorf("validatePassword(%q) err=%v wantErr=%v", c.pwd, err, c.wantErr)
		}
	}
}

func postJSON(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestSignupHandler(t *testing.T) {
	resetDB(t)

	// Wrong method
	w := httptest.NewRecorder()
	app.signupHandler(w, httptest.NewRequest(http.MethodGet, "/signup", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET signup = %d", w.Code)
	}

	// Missing fields
	w = httptest.NewRecorder()
	app.signupHandler(w, postJSON("/signup", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty signup = %d", w.Code)
	}

	// Bad JSON
	w = httptest.NewRecorder()
	app.signupHandler(w, postJSON("/signup", `{bad`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad json signup = %d", w.Code)
	}

	// Weak password
	w = httptest.NewRecorder()
	app.signupHandler(w, postJSON("/signup", `{"email":"x@y.com","password":"weak"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("weak pwd signup = %d", w.Code)
	}

	// Success
	w = httptest.NewRecorder()
	app.signupHandler(w, postJSON("/signup", `{"email":"new@y.com","password":"Abcdefgh1!"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("signup = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "session=") {
		t.Errorf("no session cookie set")
	}

	// Duplicate email -> conflict
	w = httptest.NewRecorder()
	app.signupHandler(w, postJSON("/signup", `{"email":"new@y.com","password":"Abcdefgh1!"}`))
	if w.Code != http.StatusConflict {
		t.Errorf("dup signup = %d", w.Code)
	}
}

func TestSigninHandler(t *testing.T) {
	resetDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Abcdefgh1!"), bcrypt.DefaultCost)
	_, err := app.q.CreateUser(context.Background(), dbCreateUser("signin@y.com", string(hash)))
	if err != nil {
		t.Fatal(err)
	}

	// Wrong method
	w := httptest.NewRecorder()
	app.signinHandler(w, httptest.NewRequest(http.MethodGet, "/signin", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET signin = %d", w.Code)
	}

	// Missing fields
	w = httptest.NewRecorder()
	app.signinHandler(w, postJSON("/signin", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty signin = %d", w.Code)
	}

	// Unknown user
	w = httptest.NewRecorder()
	app.signinHandler(w, postJSON("/signin", `{"email":"nope@y.com","password":"Abcdefgh1!"}`))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unknown user signin = %d", w.Code)
	}

	// Wrong password
	w = httptest.NewRecorder()
	app.signinHandler(w, postJSON("/signin", `{"email":"signin@y.com","password":"WrongPass1!"}`))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong pwd signin = %d", w.Code)
	}

	// Success
	w = httptest.NewRecorder()
	app.signinHandler(w, postJSON("/signin", `{"email":"signin@y.com","password":"Abcdefgh1!"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("signin = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["email"] != "signin@y.com" {
		t.Errorf("signin response = %+v", resp)
	}
}

func TestChangePasswordHandler(t *testing.T) {
	resetDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Abcdefgh1!"), bcrypt.DefaultCost)
	u, _ := app.q.CreateUser(context.Background(), dbCreateUser("chg@y.com", string(hash)))
	ctx := userContext(u.ID)

	// Wrong method
	w := httptest.NewRecorder()
	app.changePasswordHandler(w, httptest.NewRequest(http.MethodGet, "/change-password", nil).WithContext(ctx))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	// Missing fields
	w = httptest.NewRecorder()
	app.changePasswordHandler(w, postJSON("/change-password", `{}`).WithContext(ctx))
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty = %d", w.Code)
	}

	// Mismatched new passwords
	w = httptest.NewRecorder()
	app.changePasswordHandler(w, postJSON("/change-password", `{"currentPassword":"Abcdefgh1!","newPassword":"NewPass123!","confirmPassword":"Other123!"}`).WithContext(ctx))
	if w.Code != http.StatusBadRequest {
		t.Errorf("mismatch = %d", w.Code)
	}

	// Weak new password
	w = httptest.NewRecorder()
	app.changePasswordHandler(w, postJSON("/change-password", `{"currentPassword":"Abcdefgh1!","newPassword":"weak","confirmPassword":"weak"}`).WithContext(ctx))
	if w.Code != http.StatusBadRequest {
		t.Errorf("weak = %d", w.Code)
	}

	// Wrong current password
	w = httptest.NewRecorder()
	app.changePasswordHandler(w, postJSON("/change-password", `{"currentPassword":"WrongPass1!","newPassword":"NewPass123!","confirmPassword":"NewPass123!"}`).WithContext(ctx))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong current = %d", w.Code)
	}

	// Success
	w = httptest.NewRecorder()
	app.changePasswordHandler(w, postJSON("/change-password", `{"currentPassword":"Abcdefgh1!","newPassword":"NewPass123!","confirmPassword":"NewPass123!"}`).WithContext(ctx))
	if w.Code != http.StatusOK {
		t.Fatalf("change = %d body=%s", w.Code, w.Body.String())
	}
	// Verify new hash works
	updated, _ := app.q.GetUserByID(context.Background(), u.ID)
	if bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("NewPass123!")) != nil {
		t.Errorf("password not actually changed")
	}
}

func TestAuthMiddleware(t *testing.T) {
	resetDB(t)
	u := createTestUser(t, "mw@y.com")

	nextCalled := false
	var gotUserID int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		gotUserID = r.Context().Value(contextKey("userID")).(int64)
	})
	handler := app.authMiddleware(next)

	// No cookie
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized || nextCalled {
		t.Errorf("no cookie: code=%d nextCalled=%v", w.Code, nextCalled)
	}

	// Invalid session token
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "nonexistent"})
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token = %d", w.Code)
	}

	// Valid session: issue via issueSession
	iw := httptest.NewRecorder()
	if err := app.issueSession(iw, httptest.NewRequest(http.MethodGet, "/x", nil), u); err != nil {
		t.Fatal(err)
	}
	cookie := iw.Result().Cookies()[0]
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(w, req)
	if !nextCalled || gotUserID != u {
		t.Errorf("valid session: nextCalled=%v userID=%d want=%d", nextCalled, gotUserID, u)
	}
}
