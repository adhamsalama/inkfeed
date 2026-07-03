package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

	f := useFakeSender(t)
	serveArticleViaProxy(t, articleHTML)

	cases := []struct {
		name        string
		body        string
		wantExt     string
		wantMime    string
		wantSubUniq string // distinguishing word in the subject
	}{
		{"single epub", `{"url":"https://example.com/e1","to":"k@kindle.com","embedImages":false}`, ".epub", "application/epub+zip", "article is"},
		{"single mobi", `{"url":"https://example.com/e2","to":"k@kindle.com","format":"mobi","embedImages":false}`, ".mobi", "application/x-mobipocket-ebook", "article is"},
		{"bulk epub", `{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","author":"Bundle","embedImages":false}`, ".epub", "application/epub+zip", "articles are"},
		{"bulk mobi", `{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","format":"mobi","embedImages":false}`, ".mobi", "application/x-mobipocket-ebook", "articles are"},
	}
	for _, tc := range cases {
		w = httptest.NewRecorder()
		app.emailHandler(w, postJSON("/email", tc.body))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d body=%s", tc.name, w.Code, w.Body.String())
		}
		msg := f.last
		if msg.To != "k@kindle.com" {
			t.Errorf("%s: To = %q", tc.name, msg.To)
		}
		if !strings.Contains(msg.Subject, tc.wantSubUniq) {
			t.Errorf("%s: subject = %q, want contains %q", tc.name, msg.Subject, tc.wantSubUniq)
		}
		if len(msg.Attachments) != 1 {
			t.Fatalf("%s: attachments = %d", tc.name, len(msg.Attachments))
		}
		att := msg.Attachments[0]
		if !strings.HasSuffix(att.Filename, tc.wantExt) {
			t.Errorf("%s: filename = %q, want *%s", tc.name, att.Filename, tc.wantExt)
		}
		if att.MimeType != tc.wantMime {
			t.Errorf("%s: mime = %q, want %q", tc.name, att.MimeType, tc.wantMime)
		}
		if len(att.Content) == 0 {
			t.Errorf("%s: empty attachment content", tc.name)
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
