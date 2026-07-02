package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/mobi"
	"github.com/vincent-petithory/dataurl"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
	"golang.org/x/image/webp"
)

type MobiRequest struct {
	URL         string   `json:"url"`          // single article
	URLs        []string `json:"urls"`         // multiple articles
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"`  // optional comments page URL
	EmbedImages *bool    `json:"embedImages"`  // embed images in MOBI (default true)
}

var (
	imgAltRe       = regexp.MustCompile(`(?i)\balt="([^"]*)"`)
	mobiImgTagRe   = regexp.MustCompile(`(?i)<img\s[^>]*>`)
	mobiSrcAttr    = regexp.MustCompile(`(?i)\bsrc="([^"]*)"`)
	mobiSrcsetAttr = regexp.MustCompile(`(?i)\bsrcset="([^"]*)"`)
)

// parseSrcset returns the first (lowest-resolution) URL from a srcset value.
// Format: "url1 desc, url2 desc, ..." — first candidate is smallest/lowest quality,
// which is best for e-ink Kindle screens.
func parseSrcset(srcset string) string {
	for _, candidate := range strings.Split(srcset, ",") {
		parts := strings.Fields(strings.TrimSpace(candidate))
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

// downloadAndEmbedMobiImages fetches all images referenced in bodyHTML,
// replaces each <img> with <img recindex="N"> (1-based),
// and returns the modified HTML alongside raw image bytes for MOBI records.
// Handles absolute HTTP URLs, protocol-relative URLs (//), data URIs, and srcset.
func downloadAndEmbedMobiImages(bodyHTML string) (string, [][]byte) {
	urlToIdx := map[string]int{}
	var imageRecords [][]byte

	isHTTPURL := func(u string) bool {
		return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "//")
	}

	result := mobiImgTagRe.ReplaceAllStringFunc(bodyHTML, func(imgTag string) string {
		src := ""
		if m := mobiSrcAttr.FindStringSubmatch(imgTag); len(m) > 1 {
			src = m[1]
		}
		srcset := ""
		if m := mobiSrcsetAttr.FindStringSubmatch(imgTag); len(m) > 1 {
			srcset = m[1]
		}

		// Priority: HTTP src > srcset (skips data: placeholders) > data: URI alone
		useURL := ""
		switch {
		case isHTTPURL(src):
			useURL = src
		case srcset != "":
			useURL = parseSrcset(srcset)
		case strings.HasPrefix(src, "data:image/"):
			useURL = src
		}
		if useURL == "" {
			return imgTag
		}

		// Normalize protocol-relative URLs
		if strings.HasPrefix(useURL, "//") {
			useURL = "https:" + useURL
		}

		if idx, ok := urlToIdx[useURL]; ok {
			return mobiImgTag(imgTag, idx)
		}

		var data []byte

		if strings.HasPrefix(useURL, "data:image/") {
			du, err := dataurl.DecodeString(useURL)
			if err != nil {
				log.Printf("mobi: failed to decode data URI: %v", err)
				return imgTag
			}
			data = du.Data
		} else {
			imgReq, err := http.NewRequest("GET", useURL, nil)
			if err != nil {
				log.Printf("mobi: failed to create image request %s: %v", useURL, err)
				return imgTag
			}
			imgReq.Header.Set("User-Agent", userAgent)
			resp, err := http.DefaultClient.Do(imgReq)
			if err != nil {
				log.Printf("mobi: failed to download image %s: %v", useURL, err)
				return imgTag
			}
			defer resp.Body.Close()

			data, err = io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("mobi: failed to read image %s: %v", useURL, err)
				return imgTag
			}
		}

		ct := http.DetectContentType(data)
		if i := strings.Index(ct, ";"); i >= 0 {
			ct = strings.TrimSpace(ct[:i])
		}

		// Skip formats that are not valid Kindle image records (e.g. SVG,
		// which is detected as text/xml). Embedding them corrupts the image
		// record stream and breaks resolution of the valid images that follow.
		if ct == "image/svg+xml" || !strings.HasPrefix(ct, "image/") {
			log.Printf("mobi: skipping unsupported image type %s: %s", ct, useURL)
			return imgTag
		}

		// Convert WebP to JPEG; Kindle does not support WebP. compressImage
		// relies on golang.org/x/image/webp, which only decodes bare VP8/VP8L
		// and rejects the extended VP8X container that WordPress sites (e.g.
		// Quanta) serve, so use webpToJPEG which handles VP8X too. If the
		// conversion fails, drop the image rather than embedding a WebP the
		// Kindle renderer cannot display.
		if ct == "image/webp" {
			jpg, err := webpToJPEG(data, imageQuality())
			if err != nil {
				log.Printf("mobi: failed to convert WebP %s: %v", useURL, err)
				return imgTag
			}
			data = jpg
		}

		idx := len(imageRecords) + 1
		urlToIdx[useURL] = idx
		imageRecords = append(imageRecords, data)
		return mobiImgTag(imgTag, idx)
	})

	return result, imageRecords
}

// mobiImgTag returns an <img> tag with recindex="N" (preserving alt if present).
func mobiImgTag(original string, recindex int) string {
	alt := ""
	if m := imgAltRe.FindStringSubmatch(original); len(m) > 1 {
		alt = fmt.Sprintf(` alt="%s"`, m[1])
	}
	return fmt.Sprintf(`<img%s recindex="%d">`, alt, recindex)
}

// webpToJPEG decodes a WebP image (including the extended VP8X container) and
// re-encodes it as JPEG at the given quality. Any alpha channel is composited
// over white so transparent regions don't render as black on e-ink screens.
func webpToJPEG(data []byte, quality int) ([]byte, error) {
	src, err := decodeWebP(data)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(rgba, b, src, b.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeWebP decodes a WebP image. It first tries the simple-format decoder in
// golang.org/x/image/webp; if that fails (e.g. an extended VP8X container, which
// that package does not support), it walks the RIFF chunks itself and decodes
// the inner VP8L (lossless) or VP8 (lossy) bitstream directly.
func decodeWebP(data []byte) (image.Image, error) {
	if img, err := webp.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}

	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("webp: not a RIFF/WEBP container")
	}

	// Each RIFF chunk is a 4-byte FourCC + 4-byte little-endian size + payload,
	// padded to an even byte boundary.
	p := 12
	for p+8 <= len(data) {
		fourcc := string(data[p : p+4])
		size := int(binary.LittleEndian.Uint32(data[p+4 : p+8]))
		start := p + 8
		if start+size > len(data) {
			break
		}
		payload := data[start : start+size]
		switch fourcc {
		case "VP8L":
			return vp8l.Decode(bytes.NewReader(payload))
		case "VP8 ":
			d := vp8.NewDecoder()
			d.Init(bytes.NewReader(payload), len(payload))
			if _, err := d.DecodeFrameHeader(); err != nil {
				return nil, err
			}
			return d.DecodeFrame()
		}
		p = start + size
		if size%2 == 1 {
			p++ // skip pad byte
		}
	}
	return nil, fmt.Errorf("webp: no VP8/VP8L chunk found")
}

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)

func sanitizeFilename(s string) string {
	return strings.TrimSpace(unsafeCharsRe.ReplaceAllString(s, ""))
}

func mobiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MobiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var htmlContent string

	switch {
	case req.URL != "":
		article, err := fetchReadable(req.URL)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		if req.Title == "" {
			req.Title = article.Title
		}
		commentsHTML := fetchCommentsHTML(req.CommentsURL)
		link := `<p><a href="` + html.EscapeString(req.URL) + `">` + html.EscapeString(req.URL) + `</a></p>`
		htmlContent = "<html><body><h1>" + html.EscapeString(req.Title) + "</h1>" + link + articleMetaHTML(article) + article.Content
		if commentsHTML != "" {
			htmlContent += "<hr/><h2>Comments</h2>" + commentsHTML
		}
		htmlContent += "</body></html>"

	case len(req.URLs) > 0:
		htmlContent = fetchAndCombine(req.URLs, req.Title)

	default:
		jsonError(w, "url or urls field required", http.StatusBadRequest)
		return
	}

	embedImages := req.EmbedImages == nil || *req.EmbedImages
	var imageRecords [][]byte
	if embedImages {
		htmlContent, imageRecords = downloadAndEmbedMobiImages(htmlContent)
	}

	data, err := mobi.Write(mobi.Book{
		Title:   req.Title,
		Author:  req.Author,
		Content: htmlContent,
	}, imageRecords)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var filename string
	if len(req.URLs) > 0 {
		filename = sanitizeFilename(req.Title) + "_" + time.Now().Format("2006-01-02") + ".mobi"
	} else {
		filename = sanitizeFilename(req.Title) + ".mobi"
	}
	w.Header().Set("Content-Type", "application/x-mobipocket-ebook")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write(data)
}

// fetchAndCombine fetches all URLs concurrently (max 5 at a time) and
// returns a single HTML document combining all article contents.
func fetchAndCombine(urls []string, feedTitle string) string {
	type result struct {
		index   int
		title   string
		meta    string
		content string
		err     error
	}

	results := make([]result, len(urls))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			article, err := fetchReadable(url)
			if err != nil {
				results[idx] = result{index: idx, err: err}
				return
			}
			results[idx] = result{index: idx, title: article.Title, meta: articleMetaHTML(article), content: `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>` + article.Content}
		}(i, u)
	}
	wg.Wait()

	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h1>" + html.EscapeString(feedTitle) + "</h1><hr/>")
	for _, r := range results {
		if r.err != nil {
			sb.WriteString("<h2>[Failed to fetch article]</h2><hr/>")
		} else {
			sb.WriteString("<h2>" + html.EscapeString(r.title) + "</h2>")
			sb.WriteString(r.meta)
			sb.WriteString(r.content)
			sb.WriteString("<hr/>")
		}
	}
	sb.WriteString("</body></html>")
	return sb.String()
}
