package email

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func testServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestNewSender(t *testing.T) {
	os.Setenv("EMAIL_PROVIDER", "mailersend")
	if _, ok := NewSender().(*MailerSendSender); !ok {
		t.Error("expected MailerSendSender")
	}
	os.Setenv("EMAIL_PROVIDER", "resend")
	if _, ok := NewSender().(*ResendSender); !ok {
		t.Error("expected ResendSender")
	}
	os.Setenv("EMAIL_PROVIDER", "brevo")
	if _, ok := NewSender().(*BrevoSender); !ok {
		t.Error("expected BrevoSender")
	}
	os.Unsetenv("EMAIL_PROVIDER")
	if _, ok := NewSender().(*BrevoSender); !ok {
		t.Error("default should be BrevoSender")
	}
}

func testMsg() Message {
	return Message{
		To:          "reader@kindle.com",
		Subject:     "Test",
		HTMLContent: "<p>hi</p>",
		Attachments: []Attachment{{Filename: "a.epub", Content: []byte("data"), MimeType: "application/epub+zip"}},
	}
}

func TestBrevoSenderSuccess(t *testing.T) {
	var gotBody map[string]any
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Send(testMsg()); err != nil {
		t.Fatal(err)
	}
	if gotBody["subject"] != "Test" {
		t.Errorf("subject not sent: %+v", gotBody)
	}
}

func TestBrevoSenderError(t *testing.T) {
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"bad"}`))
	})
	orig := brevoAPIURL
	brevoAPIURL = srv.URL
	defer func() { brevoAPIURL = orig }()

	s := &BrevoSender{APIKey: "K"}
	if err := s.Send(testMsg()); err == nil {
		t.Error("expected API error")
	}
}

func TestMailerSendSender(t *testing.T) {
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	orig := mailerSendAPIURL
	mailerSendAPIURL = srv.URL
	defer func() { mailerSendAPIURL = orig }()
	s := &MailerSendSender{APIKey: "K", FromEmail: "f@x.com", FromName: "F"}
	if err := s.Send(testMsg()); err != nil {
		t.Fatal(err)
	}

	srv2 := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	})
	mailerSendAPIURL = srv2.URL
	if err := s.Send(testMsg()); err == nil {
		t.Error("expected error")
	}
}

func TestResendSender(t *testing.T) {
	var body map[string]any
	srv := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusOK)
	})
	orig := resendAPIURL
	resendAPIURL = srv.URL
	defer func() { resendAPIURL = orig }()

	s := &ResendSender{APIKey: "K", FromEmail: "f@x.com", FromName: "F"}
	if err := s.Send(testMsg()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["from"].(string), "F <f@x.com>") {
		t.Errorf("from format = %v", body["from"])
	}

	srv2 := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	resendAPIURL = srv2.URL
	if err := s.Send(testMsg()); err == nil {
		t.Error("expected error")
	}
}
