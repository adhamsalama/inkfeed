package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewEmailSender(t *testing.T) {
	os.Setenv("EMAIL_PROVIDER", "mailersend")
	if _, ok := newEmailSender().(*MailerSendSender); !ok {
		t.Error("expected MailerSendSender")
	}
	os.Setenv("EMAIL_PROVIDER", "resend")
	if _, ok := newEmailSender().(*ResendSender); !ok {
		t.Error("expected ResendSender")
	}
	os.Setenv("EMAIL_PROVIDER", "brevo")
	if _, ok := newEmailSender().(*BrevoSender); !ok {
		t.Error("expected BrevoSender")
	}
	os.Unsetenv("EMAIL_PROVIDER")
	if _, ok := newEmailSender().(*BrevoSender); !ok {
		t.Error("default should be BrevoSender")
	}
}

func emailMsg() EmailMessage {
	return EmailMessage{
		To:          "reader@kindle.com",
		Subject:     "Test",
		HTMLContent: "<p>hi</p>",
		Attachments: []EmailAttachment{{Filename: "a.epub", Content: []byte("data"), MimeType: "application/epub+zip"}},
	}
}

func TestBrevoSenderSuccess(t *testing.T) {
	var gotBody map[string]any
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer KEY" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
	})
	orig := brevoAPIURL
	brevoAPIURL = srv.URL
	defer func() { brevoAPIURL = orig }()

	s := &BrevoSender{APIKey: "KEY", FromEmail: "from@x.com", FromName: "From"}
	if err := s.Send(emailMsg()); err != nil {
		t.Fatal(err)
	}
	if gotBody["subject"] != "Test" {
		t.Errorf("subject not sent: %+v", gotBody)
	}
}

func TestBrevoSenderError(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"bad"}`))
	})
	orig := brevoAPIURL
	brevoAPIURL = srv.URL
	defer func() { brevoAPIURL = orig }()

	s := &BrevoSender{APIKey: "K"}
	if err := s.Send(emailMsg()); err == nil {
		t.Error("expected API error")
	}
}

func TestMailerSendSender(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	orig := mailerSendAPIURL
	mailerSendAPIURL = srv.URL
	defer func() { mailerSendAPIURL = orig }()
	s := &MailerSendSender{APIKey: "K", FromEmail: "f@x.com", FromName: "F"}
	if err := s.Send(emailMsg()); err != nil {
		t.Fatal(err)
	}

	// error status
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	})
	mailerSendAPIURL = srv2.URL
	if err := s.Send(emailMsg()); err == nil {
		t.Error("expected error")
	}
}

func TestResendSender(t *testing.T) {
	var body map[string]any
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
	})
	orig := resendAPIURL
	resendAPIURL = srv.URL
	defer func() { resendAPIURL = orig }()

	s := &ResendSender{APIKey: "K", FromEmail: "f@x.com", FromName: "F"}
	if err := s.Send(emailMsg()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["from"].(string), "F <f@x.com>") {
		t.Errorf("from format = %v", body["from"])
	}

	// error
	srv2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	resendAPIURL = srv2.URL
	if err := s.Send(emailMsg()); err == nil {
		t.Error("expected error")
	}
}

// emailTestServer sets up a fake email provider and the article proxy.
func setupEmailEnv(t *testing.T) {
	os.Setenv("EMAIL_PROVIDER", "brevo")
	t.Cleanup(func() { os.Unsetenv("EMAIL_PROVIDER") })
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	brevoAPIURL = srv.URL
	serveArticleViaProxy(t, articleHTML)
}

func TestEmailHandler(t *testing.T) {
	orig := brevoAPIURL
	defer func() { brevoAPIURL = orig }()

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

	setupEmailEnv(t)

	// single article, epub (default format)
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://example.com/e1","to":"k@kindle.com","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("single epub email = %d body=%s", w.Code, w.Body.String())
	}

	// single article, mobi
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"https://example.com/e2","to":"k@kindle.com","format":"mobi","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("single mobi email = %d", w.Code)
	}

	// bulk epub
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","author":"Bundle","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("bulk epub email = %d", w.Code)
	}

	// bulk mobi
	w = httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"urls":["https://example.com/a","https://example.com/b"],"to":"k@kindle.com","format":"mobi","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("bulk mobi email = %d", w.Code)
	}
}

func TestEmailHandlerFetchError(t *testing.T) {
	os.Setenv("EMAIL_PROVIDER", "brevo")
	defer os.Unsetenv("EMAIL_PROVIDER")
	setProxyURL(t, "http://127.0.0.1:0/dead")
	w := httptest.NewRecorder()
	app.emailHandler(w, postJSON("/email", `{"url":"http://127.0.0.1:0/dead","to":"k@kindle.com"}`))
	if w.Code != http.StatusBadGateway {
		t.Errorf("fetch error = %d", w.Code)
	}
}
