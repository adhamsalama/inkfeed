package export

import (
	"encoding/binary"
	"net/http"
	"strings"
	"testing"
)

func TestDownloadAndEmbedMobiImagesAllTypes(t *testing.T) {
	webpData := tinyWebP(t)
	pngData := makePNG(t, false)
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><rect width="8" height="8" fill="green"/></svg>`)

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "webp"):
			w.Write(webpData)
		case strings.Contains(r.URL.Path, "svg"):
			w.Write(svgData)
		case strings.Contains(r.URL.Path, "png"):
			w.Write(pngData)
		case strings.Contains(r.URL.Path, "text"):
			w.Write([]byte("just some plain text, not an image at all"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	html := `<img src="` + srv.URL + `/a.webp">` +
		`<img src="` + srv.URL + `/b.svg">` +
		`<img src="` + srv.URL + `/c.png">` +
		`<img src="` + srv.URL + `/d.text">` +
		`<img src="//` + strings.TrimPrefix(srv.URL, "http://") + `/e.png">`
	_, records := downloadAndEmbedMobiImages(html)
	if len(records) < 3 {
		t.Errorf("expected >=3 image records, got %d", len(records))
	}
	for i, rec := range records {
		if len(rec) < 2 || rec[0] != 0xFF || rec[1] != 0xD8 {
			t.Errorf("record %d not jpeg", i)
		}
	}

	// invalid data URI -> not embedded
	_, rec := downloadAndEmbedMobiImages(`<img src="data:image/png;base64,@@@invalid">`)
	if len(rec) != 0 {
		t.Errorf("invalid data uri should not embed: %d", len(rec))
	}
}

// TestDecodeWebPVP8XContainer drives decodeWebP's manual RIFF walker via a VP8X
// container the simple decoder rejects.
func TestDecodeWebPVP8XContainer(t *testing.T) {
	simple := tinyWebP(t)
	if string(simple[0:4]) != "RIFF" || string(simple[8:12]) != "WEBP" {
		t.Skip("unexpected tiny webp layout")
	}
	vp8lChunk := simple[12:]

	vp8x := append([]byte("VP8X"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(vp8x[4:], 10)
	vp8x = append(vp8x, make([]byte, 10)...)

	body := append([]byte("WEBP"), vp8x...)
	body = append(body, vp8lChunk...)
	riff := append([]byte("RIFF"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(riff[4:], uint32(len(body)))
	riff = append(riff, body...)

	if _, err := decodeWebP(riff); err != nil {
		t.Logf("decodeWebP VP8X err (acceptable): %v", err)
	}
}

func TestDecodeWebPWalkerBranches(t *testing.T) {
	if _, err := decodeWebP(riffChunk("VP8 ", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})); err == nil {
		t.Log("VP8 garbage unexpectedly decoded")
	}
	if _, err := decodeWebP(riffChunk("VP8L", []byte{0x2f, 1, 2, 3, 4, 5, 6, 7})); err == nil {
		t.Log("VP8L garbage unexpectedly decoded")
	}
	if _, err := decodeWebP(riffChunk("XXXX", []byte{1, 2, 3, 4})); err == nil {
		t.Error("expected error for container with no VP8 chunk")
	}
}

func riffChunk(fourcc string, payload []byte) []byte {
	chunk := append([]byte(fourcc), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(chunk[4:], uint32(len(payload)))
	chunk = append(chunk, payload...)
	if len(payload)%2 == 1 {
		chunk = append(chunk, 0)
	}
	body := append([]byte("WEBP"), chunk...)
	out := append([]byte("RIFF"), make([]byte, 4)...)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(body)))
	return append(out, body...)
}

// When an image is detected as webp/png but fails to convert, it must be dropped
// (the <img> left untouched, no record) rather than embedding bytes Kindle can't
// render.
func TestDownloadAndEmbedMobiImagesConversionFailure(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "webp"):
			// Valid RIFF/WEBP header so it sniffs as webp, but a garbage VP8 body
			// that decoding rejects.
			w.Write(append([]byte("RIFF\x0c\x00\x00\x00WEBPVP8 "), []byte("garbage")...))
		case strings.Contains(r.URL.Path, "png"):
			// PNG magic so it sniffs as png, but truncated so decode fails.
			w.Write(append([]byte("\x89PNG\r\n\x1a\n"), []byte("garbage")...))
		}
	})

	html := `<img src="` + srv.URL + `/bad.webp"><img src="` + srv.URL + `/bad.png">`
	out, records := downloadAndEmbedMobiImages(html)
	if len(records) != 0 {
		t.Errorf("unconvertible images should be dropped, got %d records", len(records))
	}
	if strings.Contains(out, "recindex") {
		t.Errorf("no image should have been embedded: %s", out)
	}
	if !strings.Contains(out, "bad.webp") || !strings.Contains(out, "bad.png") {
		t.Errorf("original <img> tags should be left intact: %s", out)
	}
}

func TestDownloadAndEmbedImagesErrorPaths(t *testing.T) {
	html := `<img src="http://127.0.0.1:0/dead.jpg">`
	out, images := downloadAndEmbedImages(html)
	if len(images) != 0 {
		t.Errorf("unreachable image should not embed: %d", len(images))
	}
	if !strings.Contains(out, "127.0.0.1:0/dead.jpg") {
		t.Errorf("original img tag should be preserved")
	}
}
