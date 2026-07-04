package export

import (
	"archive/zip"
	"bytes"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSanitizeXHTML(t *testing.T) {
	in := `<br><hr><img src="x.jpg" src="dup.jpg"><p class="a" class="b">text</p>`
	out := sanitizeXHTML(in)
	if !strings.Contains(out, "<br/>") || !strings.Contains(out, "<hr/>") {
		t.Errorf("void elements not self-closed: %s", out)
	}
	if strings.Contains(out, `src="dup.jpg"`) {
		t.Errorf("duplicate attr not removed: %s", out)
	}
}

func TestImageQuality(t *testing.T) {
	os.Unsetenv("IMAGE_QUALITY")
	if imageQuality() != 50 {
		t.Errorf("default = %d", imageQuality())
	}
	os.Setenv("IMAGE_QUALITY", "80")
	defer os.Unsetenv("IMAGE_QUALITY")
	if imageQuality() != 80 {
		t.Errorf("env = %d", imageQuality())
	}
	os.Setenv("IMAGE_QUALITY", "999") // out of range -> default
	if imageQuality() != 50 {
		t.Errorf("out of range = %d", imageQuality())
	}
	os.Setenv("IMAGE_QUALITY", "bad")
	if imageQuality() != 50 {
		t.Errorf("invalid = %d", imageQuality())
	}
}

func TestImgMediaTypeExt(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":    ".jpeg",
		"image/png":     ".png",
		"image/gif":     ".gif",
		"image/webp":    ".webp",
		"image/svg+xml": ".svg",
		"application/x": ".img",
	}
	for ct, want := range cases {
		if got := imgMediaTypeExt(ct); got != want {
			t.Errorf("imgMediaTypeExt(%q) = %q, want %q", ct, got, want)
		}
	}
}

func makeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCompressImage(t *testing.T) {
	jpg := makeJPEG(t)
	out, ct := compressImage(jpg, "image/jpeg", 10)
	if ct != "image/jpeg" {
		t.Errorf("ct = %q", ct)
	}
	if len(out) == 0 {
		t.Errorf("empty output")
	}

	// webp -> jpeg
	out, ct = compressImage(tinyWebP(t), "image/webp", 50)
	if ct != "image/jpeg" {
		t.Errorf("webp should convert to jpeg, got %q", ct)
	}
	_ = out

	// unknown type passes through
	orig := []byte{1, 2, 3}
	out, ct = compressImage(orig, "image/gif", 50)
	if ct != "image/gif" || !bytes.Equal(out, orig) {
		t.Errorf("gif should be unchanged")
	}

	// invalid jpeg data returned unchanged
	out, ct = compressImage([]byte("bad"), "image/jpeg", 50)
	if ct != "image/jpeg" || string(out) != "bad" {
		t.Errorf("invalid jpeg should pass through")
	}
}

func TestDownloadAndEmbedImages(t *testing.T) {
	jpg := makeJPEG(t)
	imgSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpg)
	})
	os.Unsetenv("IMAGE_COMPRESSION")
	html := `<img src="` + imgSrv.URL + `/a.jpg"><img src="` + imgSrv.URL + `/a.jpg">`
	out, images := downloadAndEmbedImages(html)
	if len(images) != 1 {
		t.Fatalf("expected 1 image (deduped), got %d", len(images))
	}
	if !strings.Contains(out, images[0].path) {
		t.Errorf("image path not substituted: %s", out)
	}
}

func TestGenerateEpub(t *testing.T) {
	// without images
	data, err := generateEpub("Title", "Author", "<h1>Hi</h1><p>body</p>", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
		t.Errorf("not a valid zip: %v", err)
	}

	// with images
	jpg := makeJPEG(t)
	imgSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpg)
	})
	body := `<h1>T</h1><img src="` + imgSrv.URL + `/x.jpg">`
	data, err = generateEpub("T", "", body, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Errorf("empty epub")
	}
}

func TestBuildEpubMultiArticleBody(t *testing.T) {
	body := buildEpubMultiArticleBody(fakeFetcher{}, []string{"https://example.com/1", "https://example.com/2"}, "Feed")
	if !strings.Contains(body, "Feed") || !strings.Contains(body, "Contents") {
		t.Errorf("multi body missing header/toc")
	}
	if !strings.Contains(body, `id="article-0"`) {
		t.Errorf("article anchors missing")
	}
	// failing URL -> marker
	body = buildEpubMultiArticleBody(fakeFetcher{fail: map[string]bool{"https://bad": true}}, []string{"https://good", "https://bad"}, "Mix")
	if !strings.Contains(body, "Failed to fetch article") {
		t.Errorf("expected failure marker")
	}
}

func TestEpubRenderer(t *testing.T) {
	rd := epubRenderer{}
	if rd.Ext() != "epub" || rd.Mime() == "" {
		t.Errorf("ext/mime wrong")
	}
	data, title, err := rd.Render(fakeFetcher{}, Request{URL: "https://example.com/e1", Title: "E"})
	if err != nil || len(data) == 0 || title != "E" {
		t.Fatalf("single epub: %v title=%q", err, title)
	}
	data, _, err = rd.Render(fakeFetcher{comments: "<p>c</p>"}, Request{URLs: []string{"https://a", "https://b"}, Title: "Bundle"})
	if err != nil || len(data) == 0 {
		t.Errorf("bulk epub: %v", err)
	}
	if _, _, err := rd.Render(fakeFetcher{fail: map[string]bool{"https://x": true}}, Request{URL: "https://x"}); err == nil {
		t.Error("expected fetch error")
	}
}

func TestRendererForAndFilename(t *testing.T) {
	if _, ok := RendererFor("mobi").(mobiRenderer); !ok {
		t.Error("expected mobiRenderer")
	}
	if _, ok := RendererFor("epub").(epubRenderer); !ok {
		t.Error("expected epubRenderer (default)")
	}
	if Filename("My Title", "epub", false) != "My Title.epub" {
		t.Errorf("single filename = %q", Filename("My Title", "epub", false))
	}
	if !strings.HasSuffix(Filename("T", "mobi", true), ".mobi") || !strings.Contains(Filename("T", "mobi", true), "_") {
		t.Errorf("bulk filename = %q", Filename("T", "mobi", true))
	}
}
