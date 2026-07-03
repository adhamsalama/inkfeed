package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adhamsalama/inkfeed-backend/internal/email"
)

// fakeSender records the last message and returns a configurable error, letting
// handler tests avoid any real provider round-trip.
type fakeSender struct {
	err  error
	last email.Message
}

func (f *fakeSender) Send(m email.Message) error {
	f.last = m
	return f.err
}

// useFakeSender swaps app.sender for a fake for the duration of the test.
func useFakeSender(t *testing.T) *fakeSender {
	t.Helper()
	f := &fakeSender{}
	prev := app.sender
	app.sender = f
	t.Cleanup(func() { app.sender = prev })
	return f
}

func TestEmailHandler(t *testing.T) {
	// wrong method
	w := httptest.NewRecorder()
	app.emailHandler(w, httptest.NewRequest(http.MethodGet, "/email", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}
	// bad body
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{bad`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d", w.Code)
	}
	// missing required fields
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://x"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing to = %d", w.Code)
	}

	useFakeSender(t)
	serveArticleViaProxy(t, articleHTML)

	cases := []string{
		`{"url":"https://example.com/e1","to":"k@kindle.com","embedImages":false}`,
		`{"url":"https://example.com/e2","to":"k@kindle.com","format":"mobi","embedImages":false}`,
		`{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","author":"Bundle","embedImages":false}`,
		`{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","format":"mobi","embedImages":false}`,
	}
	for _, body := range cases {
		w = httptest.NewRecorder()
		app.emailHandler(w, postJSON("/email", body))
		if w.Code != http.StatusOK {
			t.Fatalf("email %s = %d body=%s", body, w.Code, w.Body.String())
		}
	}
}

func TestEmailHandlerSendError(t *testing.T) {
	f := useFakeSender(t)
	f.err = http.ErrHandlerTimeout // any non-nil error
	serveArticleViaProxy(t, articleHTML)
	w := httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://example.com/e","to":"k@kindle.com","embedImages":false}`))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("send error = %d, want 500", w.Code)
	}
}

func TestEmailHandlerFetchError(t *testing.T) {
	useFakeSender(t)
	setProxyURL(t, "http://127.0.0.1:0/dead")
	w := httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"http://127.0.0.1:0/dead","to":"k@kindle.com"}`))
	if w.Code != http.StatusBadGateway {
		t.Errorf("fetch error = %d", w.Code)
	}
}
