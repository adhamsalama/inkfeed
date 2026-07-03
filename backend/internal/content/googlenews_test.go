package content

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestExtractBase64(t *testing.T) {
	got, err := extractBase64("https://news.google.com/articles/CBMiABC123")
	if err != nil || got != "CBMiABC123" {
		t.Errorf("articles: got %q err %v", got, err)
	}
	got, _ = extractBase64("https://news.google.com/read/CBMiXYZ")
	if got != "CBMiXYZ" {
		t.Errorf("read: got %q", got)
	}
	if _, err := extractBase64("https://example.com/articles/x"); err == nil {
		t.Error("non-google host should error")
	}
	if _, err := extractBase64("https://news.google.com/x"); err == nil {
		t.Error("short path should error")
	}
	if _, err := extractBase64("https://news.google.com/foo/bar"); err == nil {
		t.Error("wrong segment should error")
	}
	if _, err := extractBase64("://bad"); err == nil {
		t.Error("invalid url should error")
	}
}

func TestExtractDataAttrs(t *testing.T) {
	html := `<html><body><c-wiz><div jscontroller="ctrl" data-n-a-sg="SIGVAL" data-n-a-ts="TSVAL">x</div></c-wiz></body></html>`
	sig, ts := extractDataAttrs(html)
	if sig != "SIGVAL" || ts != "TSVAL" {
		t.Errorf("sig=%q ts=%q", sig, ts)
	}

	// missing attrs
	sig, ts = extractDataAttrs(`<html><c-wiz><div>no attrs</div></c-wiz></html>`)
	if sig != "" || ts != "" {
		t.Errorf("expected empty, got sig=%q ts=%q", sig, ts)
	}
}

func TestDecodeGoogleNewsURLFullFlow(t *testing.T) {
	articleSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<c-wiz><div jscontroller="c" data-n-a-sg="SIG" data-n-a-ts="TS">x</div></c-wiz>`))
	})
	batchSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		inner, _ := json.Marshal([]string{"garturlres", "https://decoded.example/final"})
		row, _ := json.Marshal([]any{"wrb.fr", "Fbv4je", string(inner)})
		outer, _ := json.Marshal([]json.RawMessage{row})
		w.Write([]byte(")]}'\n\n" + string(outer)))
	})

	origArticle := googleNewsArticleURLs
	origBatch := googleNewsBatchExecuteURL
	googleNewsArticleURLs = []string{articleSrv.URL + "/articles/%s"}
	googleNewsBatchExecuteURL = batchSrv.URL
	defer func() {
		googleNewsArticleURLs = origArticle
		googleNewsBatchExecuteURL = origBatch
	}()

	got, err := DecodeGoogleNewsURL("https://news.google.com/articles/CBMiABC")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://decoded.example/final" {
		t.Errorf("decoded = %q", got)
	}
}

func TestGetDecodingParamsFailure(t *testing.T) {
	// Server returns HTML without the data attrs -> failure.
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>nothing here</html>`))
	})
	orig := googleNewsArticleURLs
	googleNewsArticleURLs = []string{srv.URL + "/articles/%s", srv.URL + "/rss/%s"}
	defer func() { googleNewsArticleURLs = orig }()

	if _, _, err := getDecodingParams("CBMi"); err == nil {
		t.Error("expected failure fetching params")
	}
}

func TestDecodeViaAPIBadResponse(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no double newline here"))
	})
	orig := googleNewsBatchExecuteURL
	googleNewsBatchExecuteURL = srv.URL
	defer func() { googleNewsBatchExecuteURL = orig }()

	if _, err := decodeViaAPI("b64", "sig", "ts"); err == nil {
		t.Error("expected error for malformed response")
	}
}
