package server

import (
	"archive/zip"
	"bytes"
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
	if !strings.Contains(w.Header().Get("Content-Disposition"), `"My MOBI.mobi"`) {
		t.Errorf("disposition = %q", w.Header().Get("Content-Disposition"))
	}
	if w.Header().Get("Content-Type") != "application/x-mobipocket-ebook" {
		t.Errorf("mime = %q", w.Header().Get("Content-Type"))
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("BOOKMOBI")) {
		t.Errorf("body is not a MOBI file (no BOOKMOBI signature)")
	}
	// bulk
	w = httptest.NewRecorder()
	app.mobiHandler(w, postJSON("/mobi", `{"urls":["https://example.com/a","https://example.com/b"],"title":"Bundle","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("bulk mobi = %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "Bundle_") { // bulk adds a date
		t.Errorf("bulk disposition = %q", w.Header().Get("Content-Disposition"))
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
	if !strings.Contains(w.Header().Get("Content-Disposition"), `"E.epub"`) {
		t.Errorf("disposition = %q", w.Header().Get("Content-Disposition"))
	}
	// EPUB is a zip archive -> starts with the "PK" local-file signature.
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Errorf("body is not a zip/epub (no PK signature)")
	}
	if _, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len())); err != nil {
		t.Errorf("epub not a valid zip: %v", err)
	}
}
