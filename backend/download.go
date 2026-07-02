package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/mobi"
	"github.com/vincent-petithory/dataurl"
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

		// Convert WebP to JPEG; Kindle does not support WebP.
		if ct == "image/webp" {
			data, _ = compressImage(data, ct, imageQuality())
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
