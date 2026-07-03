package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"

	epub "github.com/go-shiori/go-epub"
)

type EpubRequest struct {
	URL         string   `json:"url"`
	URLs        []string `json:"urls"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"`
	EmbedImages *bool    `json:"embedImages"`
}

func (a *App) epubHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EpubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" && len(req.URLs) == 0 {
		jsonError(w, "url or urls field required", http.StatusBadRequest)
		return
	}

	er := exportRequest{
		url:         req.URL,
		urls:        req.URLs,
		commentsURL: req.CommentsURL,
		title:       req.Title,
		author:      req.Author,
		embedImages: req.EmbedImages == nil || *req.EmbedImages,
	}
	rd := epubRenderer{}
	data, title, err := rd.render(a, er)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", rd.mime())
	w.Header().Set("Content-Disposition", `attachment; filename="`+exportFilename(title, rd.ext(), er.bulk())+`"`)
	w.Write(data)
}

var (
	imgSrcRe   = regexp.MustCompile(`(?i)<img\s[^>]*\bsrc="(https?://[^"]+)"[^>]*>`)
	brRe       = regexp.MustCompile(`(?i)<br(\s[^>]*)?>`)
	hrRe       = regexp.MustCompile(`(?i)<hr(\s[^>]*)?>`)
	imgVoidRe  = regexp.MustCompile(`(?i)<img(\s[^>]*[^/])>`)
	htmlTagRe2 = regexp.MustCompile(`(?i)<[a-z][a-z0-9]*(\s[^>]*)?>`)
	attrRe     = regexp.MustCompile(`(?i)\s([a-z][a-z0-9-]*)="[^"]*"`)
)

// sanitizeXHTML fixes void HTML elements to be self-closing, as required by XHTML,
// and removes duplicate attributes which would make the document invalid XML.
func sanitizeXHTML(s string) string {
	s = brRe.ReplaceAllString(s, "<br/>")
	s = hrRe.ReplaceAllString(s, "<hr/>")
	s = imgVoidRe.ReplaceAllString(s, "<img$1/>")
	s = htmlTagRe2.ReplaceAllStringFunc(s, func(tag string) string {
		seen := map[string]bool{}
		return attrRe.ReplaceAllStringFunc(tag, func(attr string) string {
			name := attrRe.FindStringSubmatch(attr)[1]
			key := strings.ToLower(name)
			if seen[key] {
				return ""
			}
			seen[key] = true
			return attr
		})
	})
	return s
}

type embeddedImage struct {
	path      string // relative to OEBPS/, e.g. "images/img0.jpeg"
	mediaType string
	data      []byte
}

// imageQuality returns the JPEG compression quality (1–100) from the
// IMAGE_QUALITY env var, defaulting to 50.
func imageQuality() int {
	if v := os.Getenv("IMAGE_QUALITY"); v != "" {
		if q, err := strconv.Atoi(v); err == nil && q >= 1 && q <= 100 {
			return q
		}
	}
	return 50
}

// compressImage re-encodes a JPEG at the given quality (1–100), and converts
// WebP to JPEG for compatibility with Amazon's conversion service.
// Other formats are returned unchanged.
func compressImage(data []byte, mediaType string, quality int) ([]byte, string) {
	switch mediaType {
	case "image/jpeg":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return data, mediaType
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return data, mediaType
		}
		if buf.Len() >= len(data) {
			return data, mediaType
		}
		return buf.Bytes(), mediaType
	case "image/webp":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return data, mediaType
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return data, mediaType
		}
		return buf.Bytes(), "image/jpeg"
	default:
		return data, mediaType
	}
}

func downloadAndEmbedImages(bodyHTML string) (string, []embeddedImage) {
	urlToIdx := map[string]int{}
	var images []embeddedImage

	result := imgSrcRe.ReplaceAllStringFunc(bodyHTML, func(match string) string {
		sub := imgSrcRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		srcURL := sub[1]

		if idx, ok := urlToIdx[srcURL]; ok {
			return strings.Replace(match, `src="`+srcURL+`"`, `src="`+images[idx].path+`"`, 1)
		}

		imgReq, err := http.NewRequest("GET", srcURL, nil)
		if err != nil {
			log.Printf("epub: failed to create image request %s: %v", srcURL, err)
			return match
		}
		imgReq.Header.Set("User-Agent", userAgent)
		resp, err := http.DefaultClient.Do(imgReq)
		if err != nil {
			log.Printf("epub: failed to download image %s: %v", srcURL, err)
			return match
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("epub: failed to read image %s: %v", srcURL, err)
			return match
		}

		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		if i := strings.Index(ct, ";"); i >= 0 {
			ct = strings.TrimSpace(ct[:i])
		}

		if os.Getenv("IMAGE_COMPRESSION") != "false" {
			data, ct = compressImage(data, ct, imageQuality())
		}
		ext := imgMediaTypeExt(ct)
		imgPath := fmt.Sprintf("images/img%d%s", len(images), ext)

		urlToIdx[srcURL] = len(images)
		images = append(images, embeddedImage{path: imgPath, mediaType: ct, data: data})

		return strings.Replace(match, `src="`+srcURL+`"`, `src="`+imgPath+`"`, 1)
	})

	return result, images
}

func imgMediaTypeExt(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpeg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".img"
	}
}

func generateEpub(title, author, bodyHTML string, embedImages bool) ([]byte, error) {
	e, err := epub.NewEpub(title)
	if err != nil {
		return nil, err
	}
	if author != "" {
		e.SetAuthor(author)
	}

	if embedImages {
		var images []embeddedImage
		bodyHTML, images = downloadAndEmbedImages(bodyHTML)

		if len(images) > 0 {
			tmpDir, err := os.MkdirTemp("", "epub-images-*")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(tmpDir)

			for _, img := range images {
				tmpFile := filepath.Join(tmpDir, filepath.Base(img.path))
				if err := os.WriteFile(tmpFile, img.data, 0644); err != nil {
					log.Printf("epub: failed to write temp image: %v", err)
					continue
				}
				epubImgPath, err := e.AddImage(tmpFile, filepath.Base(img.path))
				if err != nil {
					log.Printf("epub: failed to add image: %v", err)
					continue
				}
				bodyHTML = strings.ReplaceAll(bodyHTML, `src="`+img.path+`"`, `src="`+epubImgPath+`"`)
			}
		}
	} else {
		bodyHTML = imgSrcRe.ReplaceAllString(bodyHTML, "")
	}

	bodyHTML = sanitizeXHTML(bodyHTML)

	if _, err := e.AddSection(bodyHTML, title, "", ""); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := e.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
