package export

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
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

func TestMobiRenderer(t *testing.T) {
	rd := mobiRenderer{}
	if rd.Ext() != "mobi" || rd.Mime() == "" {
		t.Errorf("ext/mime wrong")
	}
	data, title, err := rd.Render(fakeFetcher{}, Request{URL: "https://example.com/m1", Title: "My MOBI"})
	if err != nil || len(data) == 0 || title != "My MOBI" {
		t.Fatalf("single mobi: %v title=%q len=%d", err, title, len(data))
	}
	data, _, err = rd.Render(fakeFetcher{}, Request{URLs: []string{"https://a", "https://b"}, Title: "Bundle", EmbedImages: false})
	if err != nil || len(data) == 0 {
		t.Errorf("bulk mobi: %v", err)
	}
	if _, _, err := rd.Render(fakeFetcher{fail: map[string]bool{"https://x": true}}, Request{URL: "https://x"}); err == nil {
		t.Error("expected fetch error")
	}
}

func TestFetchAndCombine(t *testing.T) {
	out := fetchAndCombine(fakeFetcher{}, []string{"https://example.com/1", "https://example.com/2"}, "Feed Title")
	if !strings.Contains(out, "Feed Title") || !strings.Contains(out, "Contents") {
		t.Errorf("combined output missing header/toc")
	}
	if strings.Count(out, "inkfeed-toc-") < 2 {
		t.Errorf("expected per-article anchors")
	}

	// a failing URL yields the failure marker
	out = fetchAndCombine(fakeFetcher{fail: map[string]bool{"https://bad": true}}, []string{"https://good", "https://bad"}, "Mix")
	if !strings.Contains(out, "Failed to fetch article") {
		t.Errorf("expected failure marker")
	}
}
