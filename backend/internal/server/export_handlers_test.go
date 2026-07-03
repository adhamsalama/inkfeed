package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMobiHandler(t *testing.T) {
	// wrong method
	w := httptest.NewRecorder()
	app.mobiHandler(w, httptest.NewRequest(http.MethodGet, "/mobi", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}
	// bad body
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{bad`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d", w.Code)
	}
	// no url
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("no url = %d", w.Code)
	}

	serveArticleViaProxy(t, articleHTML)
	// single
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{"url":"https://example.com/m1","title":"My MOBI","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("single mobi = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), ".mobi") {
		t.Errorf("no mobi filename")
	}
	// bulk
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{"urls":["https://example.com/a","https://example.com/b"],"title":"Bundle","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("bulk mobi = %d", w.Code)
	}

	// fetch error -> 502
	setProxyURL(t, "http://127.0.0.1:0/dead")
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{"url":"http://127.0.0.1:0/dead","embedImages":false}`))
	if w.Code != http.StatusBadGateway {
		t.Errorf("mobi fetch error = %d", w.Code)
	}
}

func TestEpubHandler(t *testing.T) {
	w := httptest.NewRecorder()
	app.epubHandler(w, httptest.NewRequest(http.MethodGet, "/epub", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}
	w = httptest.NewRecorder()
	app.epubHandler(w, postJSON("/epub", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("no url = %d", w.Code)
	}

	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	app.epubHandler(w, postJSON("/epub", `{"url":"https://example.com/e1","title":"E","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("single epub = %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), ".epub") {
		t.Errorf("no epub filename")
	}
}
