package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSrcset(t *testing.T) {
	if got := parseSrcset("a.jpg 480w, b.jpg 800w"); got != "a.jpg" {
		t.Errorf("parseSrcset = %q", got)
	}
	if got := parseSrcset("only.jpg"); got != "only.jpg" {
		t.Errorf("single = %q", got)
	}
	if got := parseSrcset(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestResponsiveVariantKey(t *testing.T) {
	if responsiveVariantKey("https://x/photo.jpg") != "" {
		t.Errorf("no marker should be empty")
	}
	d := responsiveVariantKey("https://x/photo-Desktop.jpg")
	m := responsiveVariantKey("https://x/photo-Mobile.jpg")
	if d == "" || d != m {
		t.Errorf("desktop/mobile should collapse: %q vs %q", d, m)
	}
}

func TestMobiImgTag(t *testing.T) {
	if got := mobiImgTag(`<img src="x" alt="hello">`, 3); got != `<img alt="hello" recindex="3">` {
		t.Errorf("with alt = %q", got)
	}
	if got := mobiImgTag(`<img src="x">`, 1); got != `<img recindex="1">` {
		t.Errorf("no alt = %q", got)
	}
}

func TestInsertJFIFHeader(t *testing.T) {
	// non-jpeg untouched
	if got := insertJFIFHeader([]byte{1, 2, 3}); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("non-jpeg changed")
	}
	// already has APP0
	app0 := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 1}
	if got := insertJFIFHeader(app0); !bytes.Equal(got, app0) {
		t.Errorf("existing APP0 changed")
	}
	// SOI + DQT -> APP0 inserted
	in := []byte{0xFF, 0xD8, 0xFF, 0xDB, 0x00}
	out := insertJFIFHeader(in)
	if out[2] != 0xFF || out[3] != 0xE0 {
		t.Errorf("APP0 not inserted: %v", out[:4])
	}
}

func TestIsSVG(t *testing.T) {
	if !isSVG([]byte(`<?xml version="1.0"?><svg xmlns="..."></svg>`)) {
		t.Error("should detect svg")
	}
	if isSVG([]byte(`<html></html>`)) {
		t.Error("html is not svg")
	}
	// large head
	big := append([]byte(strings.Repeat("x", 2000)), []byte("<svg")...)
	if isSVG(big) {
		t.Error("svg past 1024 window should not be detected")
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("Hello, World! 2024/03"); got != "Hello World 202403" {
		t.Errorf("sanitizeFilename = %q", got)
	}
	if got := sanitizeFilename("  trim me  "); got != "trim me" {
		t.Errorf("trim = %q", got)
	}
}

func makePNG(t *testing.T, transparent bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	c := color.RGBA{255, 0, 0, 255}
	if transparent {
		c = color.RGBA{0, 0, 255, 128}
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPNGToJPEG(t *testing.T) {
	jpg, err := pngToJPEG(makePNG(t, true), 50)
	if err != nil {
		t.Fatal(err)
	}
	if jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Errorf("not a jpeg: %v", jpg[:2])
	}
	if _, err := pngToJPEG([]byte("not a png"), 50); err == nil {
		t.Error("expected decode error")
	}
}

// tinyWebP is a 1x1 lossless (VP8L) WebP image.
const tinyWebPB64 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func tinyWebP(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(tinyWebPB64)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeWebP(t *testing.T) {
	img, err := decodeWebP(tinyWebP(t))
	if err != nil {
		t.Fatalf("decodeWebP: %v", err)
	}
	if img.Bounds().Dx() != 1 {
		t.Errorf("unexpected dimensions: %v", img.Bounds())
	}

	// not a webp
	if _, err := decodeWebP([]byte("nope")); err == nil {
		t.Error("expected error for non-webp")
	}
	// RIFF/WEBP with no VP8 chunk
	bad := append([]byte("RIFF\x04\x00\x00\x00WEBP"), 0, 0, 0, 0)
	if _, err := decodeWebP(bad); err == nil {
		t.Error("expected error for missing VP8 chunk")
	}
}

func TestWebPToJPEG(t *testing.T) {
	jpg, err := webpToJPEG(tinyWebP(t), 50)
	if err != nil {
		t.Fatal(err)
	}
	if jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Errorf("not jpeg")
	}
	if _, err := webpToJPEG([]byte("bad"), 50); err == nil {
		t.Error("expected error")
	}
}

func TestSVGToJPEG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>`)
	jpg, err := svgToJPEG(svg, 50)
	if err != nil {
		t.Fatal(err)
	}
	if jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Errorf("not jpeg")
	}
	// SVG without viewBox uses default dimensions
	svg2 := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="5" height="5"/></svg>`)
	if _, err := svgToJPEG(svg2, 50); err != nil {
		t.Errorf("default-size svg: %v", err)
	}
}

func TestDownloadAndEmbedMobiImages(t *testing.T) {
	pngData := makePNG(t, false)
	imgSrv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	})

	html := `<p>text</p><img src="` + imgSrv.URL + `/a.png"><img src="` + imgSrv.URL + `/a.png">`
	out, records := downloadAndEmbedMobiImages(html)
	if len(records) != 1 {
		t.Fatalf("expected 1 image record (deduped), got %d", len(records))
	}
	if !strings.Contains(out, `recindex="1"`) {
		t.Errorf("recindex not inserted: %s", out)
	}

	// data URI image
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)
	out2, rec2 := downloadAndEmbedMobiImages(`<img src="` + dataURI + `">`)
	if len(rec2) != 1 {
		t.Errorf("data uri image not embedded: %d", len(rec2))
	}
	_ = out2

	// srcset only
	out3, rec3 := downloadAndEmbedMobiImages(`<img srcset="` + imgSrv.URL + `/s.png 480w">`)
	if len(rec3) != 1 {
		t.Errorf("srcset image not embedded: %d", len(rec3))
	}
	_ = out3

	// no usable src -> unchanged, no records
	out4, rec4 := downloadAndEmbedMobiImages(`<img alt="x">`)
	if len(rec4) != 0 || !strings.Contains(out4, "<img") {
		t.Errorf("no-src image mishandled")
	}
}

func TestAnnotateArticleHeadings(t *testing.T) {
	content := `<h1>Intro</h1><p>a</p><h2>Body</h2><p>b</p><h3></h3>`
	out, labels := annotateArticleHeadings(content, 0)
	if len(labels) != 2 || labels[0] != "Intro" || labels[1] != "Body" {
		t.Errorf("labels = %v", labels)
	}
	if strings.Count(out, "inkfeed-toc-") != 2 {
		t.Errorf("anchors = %d", strings.Count(out, "inkfeed-toc-"))
	}
}

func TestBuildMobiTOC(t *testing.T) {
	if buildMobiTOC("<p>no anchors</p>") != nil {
		t.Error("expected nil for no anchors")
	}
	html := `<a name="inkfeed-toc-0"></a><h2>Chapter A</h2><a name="inkfeed-toc-1"></a><h2>Chapter B</h2>`
	toc := buildMobiTOC(html)
	if len(toc) != 2 || toc[0].Label != "Chapter A" || toc[1].Label != "Chapter B" {
		t.Errorf("toc = %+v", toc)
	}
}

func TestPatchMobiTOCFilepos(t *testing.T) {
	// no anchors -> unchanged
	in := `<a filepos="` + mobiTOCPlaceholder + `">x</a>`
	if patchMobiTOCFilepos("no placeholders here") != "no placeholders here" {
		t.Error("no-anchor content changed")
	}
	html := `<ul><li><a filepos="` + mobiTOCPlaceholder + `">A</a></li></ul><a name="inkfeed-toc-0"></a><h2>A</h2>`
	out := patchMobiTOCFilepos(html)
	if strings.Contains(out, mobiTOCPlaceholder) {
		t.Errorf("placeholder not patched: %s", out)
	}
	_ = in
}

func TestMobiHandler(t *testing.T) {
	// wrong method
	w := httptest.NewRecorder()
	mobiHandler(w, httptest.NewRequest(http.MethodGet, "/mobi", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d", w.Code)
	}

	// bad body
	w = httptest.NewRecorder()
	mobiHandler(w, postJSON("/mobi", `{bad`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad body = %d", w.Code)
	}

	// neither url nor urls
	w = httptest.NewRecorder()
	mobiHandler(w, postJSON("/mobi", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("no url = %d", w.Code)
	}

	// single url
	serveArticleViaProxy(t, articleHTML)
	w = httptest.NewRecorder()
	mobiHandler(w, postJSON("/mobi", `{"url":"https://example.com/m1","title":"My MOBI","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("single mobi = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), ".mobi") {
		t.Errorf("no mobi filename")
	}

	// multiple urls
	w = httptest.NewRecorder()
	mobiHandler(w, postJSON("/mobi", `{"urls":["https://example.com/a","https://example.com/b"],"title":"Bundle","embedImages":false}`))
	if w.Code != http.StatusOK {
		t.Errorf("multi mobi = %d", w.Code)
	}
}

func TestFetchAndCombine(t *testing.T) {
	serveArticleViaProxy(t, articleHTML)
	out := fetchAndCombine([]string{"https://example.com/1", "https://example.com/2"}, "Feed Title")
	if !strings.Contains(out, "Feed Title") || !strings.Contains(out, "Contents") {
		t.Errorf("combined output missing header/toc")
	}
	if strings.Count(out, "inkfeed-toc-") < 2 {
		t.Errorf("expected per-article anchors")
	}
}
