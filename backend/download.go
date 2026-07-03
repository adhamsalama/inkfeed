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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/mobi"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"github.com/vincent-petithory/dataurl"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
	"golang.org/x/image/webp"
)

type MobiRequest struct {
	URL         string   `json:"url"`  // single article
	URLs        []string `json:"urls"` // multiple articles
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"` // optional comments page URL
	EmbedImages *bool    `json:"embedImages"` // embed images in MOBI (default true)
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
	variantSeen := map[string]bool{}
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

		// Drop responsive duplicates: sites (e.g. Quanta) ship separate
		// "Desktop" and "Mobile" variants of the same figure that CSS media
		// queries would hide, but without CSS both would render. Keep the first
		// variant seen and drop the rest.
		if key := responsiveVariantKey(useURL); key != "" {
			if variantSeen[key] {
				return ""
			}
			variantSeen[key] = true
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

		// Rasterize SVG to JPEG; Kindle cannot render SVG image records, and
		// embedding one verbatim corrupts the image record stream. SVG is
		// detected as text/xml by http.DetectContentType, so sniff the bytes.
		if ct == "image/svg+xml" || isSVG(data) {
			jpg, err := svgToJPEG(data, imageQuality())
			if err != nil {
				log.Printf("mobi: failed to rasterize SVG %s: %v", useURL, err)
				return imgTag
			}
			data = jpg
			ct = "image/jpeg"
		} else if !strings.HasPrefix(ct, "image/") {
			// Anything else that isn't an image is not a valid Kindle image
			// record; embedding it breaks resolution of the images that follow.
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

		// Convert PNG to JPEG; the KF7 MOBI inline image renderer does not
		// display PNG (it only shows as a cover), so re-encode to JPEG. If the
		// conversion fails, drop the image rather than embedding a PNG that
		// won't render inline.
		if ct == "image/png" {
			jpg, err := pngToJPEG(data, imageQuality())
			if err != nil {
				log.Printf("mobi: failed to convert PNG %s: %v", useURL, err)
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

// responsiveVariantKey returns a normalized key identifying a responsive image
// variant, or "" if the URL carries no responsive marker (so it never
// participates in dedup). It strips the "desktop"/"mobile" tokens so the two
// variants of the same figure collapse to the same key.
func responsiveVariantKey(u string) string {
	lower := strings.ToLower(u)
	if !strings.Contains(lower, "desktop") && !strings.Contains(lower, "mobile") {
		return ""
	}
	return strings.NewReplacer("desktop", "", "mobile", "").Replace(lower)
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
	return insertJFIFHeader(buf.Bytes()), nil
}

// insertJFIFHeader inserts a standard JFIF APP0 segment immediately after the
// SOI marker. Go's image/jpeg encoder omits the APP0 header (it writes SOI
// straight into a quantization table, FF D8 FF DB), and Kindle's JPEG decoder
// rejects such images inline. Prepending the APP0 marker yields the FF D8 FF E0
// framing Kindle expects.
func insertJFIFHeader(j []byte) []byte {
	if len(j) < 2 || j[0] != 0xFF || j[1] != 0xD8 {
		return j // not a JPEG; leave untouched
	}
	if len(j) >= 4 && j[2] == 0xFF && j[3] == 0xE0 {
		return j // already has an APP0 segment
	}
	// FF E0, length 16, "JFIF\0", version 1.1, units=0, X/Y density 1, no thumb.
	app0 := []byte{
		0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00,
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
	}
	out := make([]byte, 0, len(j)+len(app0))
	out = append(out, j[0], j[1])
	out = append(out, app0...)
	out = append(out, j[2:]...)
	return out
}

// pngToJPEG decodes a PNG and re-encodes it as JPEG at the given quality. Any
// alpha channel is composited over white so transparent regions don't render
// as black on e-ink screens, and a JFIF APP0 header is added so Kindle accepts
// the result inline.
func pngToJPEG(data []byte, quality int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
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
	return insertJFIFHeader(buf.Bytes()), nil
}

// isSVG reports whether data looks like an SVG document. http.DetectContentType
// classifies SVG as text/xml or text/plain, so sniff for an <svg element near
// the start (after an optional XML declaration / comments / doctype).
func isSVG(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// svgToJPEG rasterizes an SVG (at its intrinsic viewBox size, default 800x600)
// onto a white background and encodes it as JPEG with a JFIF APP0 header so
// Kindle renders it inline.
func svgToJPEG(data []byte, quality int) ([]byte, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 600
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	icon.SetTarget(0, 0, float64(w), float64(h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return insertJFIFHeader(buf.Bytes()), nil
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

func (a *App) mobiHandler(w http.ResponseWriter, r *http.Request) {
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
		article, err := a.fetchReadable(req.URL)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadGateway)
			return
		}
		if req.Title == "" {
			req.Title = article.Title
		}
		commentsHTML := a.fetchCommentsHTML(req.CommentsURL)
		link := `<p><a href="` + html.EscapeString(req.URL) + `">` + html.EscapeString(req.URL) + `</a></p>`

		// Build a table of contents from the article's section headings (plus a
		// Comments entry when present). Only worthwhile when there are at least
		// two navigation points; otherwise fall back to a plain document.
		annotated, labels := annotateArticleHeadings(article.Content, 0)
		hasComments := commentsHTML != ""
		total := len(labels)
		if hasComments {
			total++
		}

		var sb strings.Builder
		sb.WriteString("<html><body><h1>" + html.EscapeString(req.Title) + "</h1>" + link + articleMetaHTML(article))
		if total >= 2 {
			sb.WriteString("<h2>Contents</h2><ul>")
			for _, l := range labels {
				sb.WriteString(fmt.Sprintf(`<li><a filepos="%s">%s</a></li>`, mobiTOCPlaceholder, html.EscapeString(l)))
			}
			if hasComments {
				sb.WriteString(fmt.Sprintf(`<li><a filepos="%s">Comments</a></li>`, mobiTOCPlaceholder))
			}
			sb.WriteString("</ul><mbp:pagebreak/><hr/>")
			sb.WriteString(annotated)
			if hasComments {
				sb.WriteString(fmt.Sprintf(`<hr/><a name="inkfeed-toc-%d"></a><h2>Comments</h2>`, len(labels)) + commentsHTML)
			}
		} else {
			sb.WriteString(article.Content)
			if hasComments {
				sb.WriteString("<hr/><h2>Comments</h2>" + commentsHTML)
			}
		}
		sb.WriteString("</body></html>")
		htmlContent = sb.String()

	case len(req.URLs) > 0:
		htmlContent = a.fetchAndCombine(req.URLs, req.Title)

	default:
		jsonError(w, "url or urls field required", http.StatusBadRequest)
		return
	}

	embedImages := req.EmbedImages == nil || *req.EmbedImages
	var imageRecords [][]byte
	if embedImages {
		htmlContent, imageRecords = downloadAndEmbedMobiImages(htmlContent)
	}

	// Resolve the table-of-contents filepos links to their byte offsets. This
	// must run last, after image embedding has shifted the final byte layout.
	htmlContent = patchMobiTOCFilepos(htmlContent)

	data, err := mobi.Write(mobi.Book{
		Title:   req.Title,
		Author:  req.Author,
		Content: htmlContent,
		TOC:     buildMobiTOC(htmlContent),
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
func (a *App) fetchAndCombine(urls []string, feedTitle string) string {
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
			article, err := a.fetchReadable(url)
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
	sb.WriteString("<h1>" + html.EscapeString(feedTitle) + "</h1>")

	// Table of contents: one filepos link per article. The filepos values are
	// placeholders here and are rewritten to real byte offsets by
	// patchMobiTOCFilepos once the final HTML layout is known. Each link points
	// at the matching <a name="inkfeed-toc-N"> anchor emitted before its article.
	sb.WriteString("<h2>Contents</h2><ul>")
	for _, r := range results {
		title := r.title
		if r.err != nil || title == "" {
			title = "[Failed to fetch article]"
		}
		sb.WriteString(fmt.Sprintf(`<li><a filepos="%s">%s</a></li>`, mobiTOCPlaceholder, html.EscapeString(title)))
	}
	sb.WriteString("</ul><mbp:pagebreak/><hr/>")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf(`<a name="inkfeed-toc-%d"></a>`, i))
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

// mobiTOCPlaceholder is a fixed-width (10-digit) filepos value emitted by
// fetchAndCombine. patchMobiTOCFilepos rewrites each occurrence in place with a
// real byte offset; keeping the width fixed means the rewrite never shifts the
// byte layout, so offsets computed on the assembled document stay valid.
const mobiTOCPlaceholder = "0000000000"

var (
	mobiTOCAnchorRe = regexp.MustCompile(`<a name="inkfeed-toc-(\d+)"></a>`)
	mobiTOCLabelRe  = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	mobiTagStripRe  = regexp.MustCompile(`<[^>]*>`)
	mobiHeadingRe   = regexp.MustCompile(`(?is)<h[1-4][^>]*>(.*?)</h[1-4]>`)
)

// annotateArticleHeadings inserts a TOC anchor (<a name="inkfeed-toc-N">) before
// each heading in an article body, numbering from startIdx, and returns the
// annotated HTML along with the ordered heading labels. It powers the
// single-article table of contents.
func annotateArticleHeadings(content string, startIdx int) (string, []string) {
	var labels []string
	var b strings.Builder
	last := 0
	idx := startIdx
	for _, m := range mobiHeadingRe.FindAllStringSubmatchIndex(content, -1) {
		label := strings.TrimSpace(html.UnescapeString(mobiTagStripRe.ReplaceAllString(content[m[2]:m[3]], "")))
		if label == "" {
			continue
		}
		b.WriteString(content[last:m[0]])
		b.WriteString(fmt.Sprintf(`<a name="inkfeed-toc-%d"></a>`, idx))
		last = m[0]
		labels = append(labels, label)
		idx++
	}
	b.WriteString(content[last:])
	return b.String(), labels
}

// buildMobiTOC extracts NCX navigation points from the finalized HTML: one per
// <a name="inkfeed-toc-N"> anchor, at that anchor's byte offset, labelled with
// the text of the <h2> heading that follows it. The byte offsets match the
// filepos values patched into the inline Contents list, so the device TOC and
// the inline links land in the same place.
func buildMobiTOC(htmlContent string) []mobi.TOCEntry {
	matches := mobiTOCAnchorRe.FindAllStringIndex(htmlContent, -1)
	if len(matches) == 0 {
		return nil
	}
	toc := make([]mobi.TOCEntry, 0, len(matches))
	for _, m := range matches {
		label := "Article"
		if lm := mobiTOCLabelRe.FindStringSubmatch(htmlContent[m[1]:]); lm != nil {
			if text := strings.TrimSpace(html.UnescapeString(mobiTagStripRe.ReplaceAllString(lm[1], ""))); text != "" {
				label = text
			}
		}
		toc = append(toc, mobi.TOCEntry{Offset: m[0], Label: label})
	}
	return toc
}

// patchMobiTOCFilepos rewrites the placeholder filepos values in the table of
// contents to the byte offsets of their matching <a name="inkfeed-toc-N">
// anchors. MOBI filepos values are byte offsets into the (uncompressed) text
// stream, which is exactly the HTML byte string here. The k-th filepos link
// (in document order) targets anchor k, matching fetchAndCombine's emission
// order.
func patchMobiTOCFilepos(htmlContent string) string {
	// Map anchor index -> byte offset of the anchor.
	offsets := map[int]int{}
	for _, m := range mobiTOCAnchorRe.FindAllStringSubmatchIndex(htmlContent, -1) {
		idx, err := strconv.Atoi(htmlContent[m[2]:m[3]])
		if err != nil {
			continue
		}
		offsets[idx] = m[0]
	}
	if len(offsets) == 0 {
		return htmlContent
	}

	needle := `filepos="` + mobiTOCPlaceholder + `"`
	var sb strings.Builder
	rest := htmlContent
	k := 0
	for {
		i := strings.Index(rest, needle)
		if i < 0 {
			sb.WriteString(rest)
			break
		}
		sb.WriteString(rest[:i])
		offset := offsets[k]
		sb.WriteString(fmt.Sprintf(`filepos="%010d"`, offset))
		rest = rest[i+len(needle):]
		k++
	}
	return sb.String()
}
