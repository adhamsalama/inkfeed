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
)

type MobiRequest struct {
	URL         string   `json:"url"`          // single article
	URLs        []string `json:"urls"`         // multiple articles
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"`  // optional comments page URL
	EmbedImages *bool    `json:"embedImages"`  // embed images in MOBI (default true)
}

var imgAltRe = regexp.MustCompile(`(?i)\balt="([^"]*)"`)

// downloadAndEmbedMobiImages fetches all images referenced in bodyHTML,
// replaces each <img src="URL" ...> with <img recindex="N"> (1-based),
// and returns the modified HTML alongside raw image bytes for MOBI records.
// WebP images are converted to JPEG for Kindle compatibility.
func downloadAndEmbedMobiImages(bodyHTML string) (string, [][]byte) {
	urlToIdx := map[string]int{} // url → 1-based record index
	var imageRecords [][]byte

	result := imgSrcRe.ReplaceAllStringFunc(bodyHTML, func(match string) string {
		sub := imgSrcRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		srcURL := sub[1]

		if idx, ok := urlToIdx[srcURL]; ok {
			return mobiImgTag(match, idx)
		}

		imgReq, err := http.NewRequest("GET", srcURL, nil)
		if err != nil {
			log.Printf("mobi: failed to create image request %s: %v", srcURL, err)
			return match
		}
		imgReq.Header.Set("User-Agent", userAgent)
		resp, err := http.DefaultClient.Do(imgReq)
		if err != nil {
			log.Printf("mobi: failed to download image %s: %v", srcURL, err)
			return match
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("mobi: failed to read image %s: %v", srcURL, err)
			return match
		}

		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		if i := strings.Index(ct, ";"); i >= 0 {
			ct = strings.TrimSpace(ct[:i])
		}

		// Convert WebP to JPEG; Kindle does not support WebP.
		if ct == "image/webp" {
			data, ct = compressImage(data, ct, imageQuality())
		}

		idx := len(imageRecords) + 1 // 1-based
		urlToIdx[srcURL] = idx
		imageRecords = append(imageRecords, data)

		_ = ct // ct used implicitly via the record; Kindle infers type from bytes
		return mobiImgTag(match, idx)
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
