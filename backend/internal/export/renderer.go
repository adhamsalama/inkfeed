// Package export renders a set of source articles into a downloadable e-reader
// document (EPUB or MOBI). Each format is a Renderer; content acquisition is
// injected via the Fetcher interface so this package never depends on the HTTP
// or content-scraping layers directly.
package export

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/internal/content"
	"github.com/adhamsalama/inkfeed-backend/mobi"
	readability "github.com/go-shiori/go-readability"
)

// Fetcher supplies the article content an export needs. content.Service
// satisfies it; the interface is declared here (the consumer) to keep the
// dependency arrow pointing export → content.
type Fetcher interface {
	FetchReadable(rawURL string) (readability.Article, error)
	FetchComments(rawURL string) string
}

// Request is the format-agnostic description of an export job. A single article
// (URL + optional CommentsURL) and a bulk export (URLs) share the same shape.
type Request struct {
	URL         string
	URLs        []string
	CommentsURL string
	Title       string
	Author      string
	EmbedImages bool
}

func (req Request) bulk() bool { return len(req.URLs) > 0 }

// Renderer turns a Request into a downloadable document. mobiRenderer and
// epubRenderer are the implementations; a new output format means one more
// Renderer, not another branch in the handlers.
type Renderer interface {
	Render(f Fetcher, req Request) (data []byte, title string, err error)
	Ext() string
	Mime() string
}

// RendererFor maps a "format" field to a Renderer (epub is the default).
func RendererFor(format string) Renderer {
	if format == "mobi" {
		return mobiRenderer{}
	}
	return epubRenderer{}
}

// Filename builds the download filename: "<title>.<ext>" for a single article,
// "<title>_<date>.<ext>" for a bulk export.
func Filename(title, ext string, bulk bool) string {
	name := sanitizeFilename(title)
	if bulk {
		return name + "_" + time.Now().Format("2006-01-02") + "." + ext
	}
	return name + "." + ext
}

// FilenameForRequest is Filename with the request's bulk-ness applied.
func FilenameForRequest(req Request, title, ext string) string {
	return Filename(title, ext, req.bulk())
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// fetchedArticle is the raw material produced by acquireArticles.
type fetchedArticle struct {
	url     string
	title   string
	meta    string
	content string // link line + readable HTML
	err     error
}

// acquireArticles fetches every URL concurrently (max 5 in flight), preserving
// input order — the single content-acquisition path shared by both bulk formats.
func acquireArticles(f Fetcher, urls []string) []fetchedArticle {
	out := make([]fetchedArticle, len(urls))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			article, err := f.FetchReadable(url)
			if err != nil {
				out[idx] = fetchedArticle{url: url, err: err}
				return
			}
			out[idx] = fetchedArticle{
				url:     url,
				title:   article.Title,
				meta:    content.ArticleMeta(article),
				content: `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>` + article.Content,
			}
		}(i, u)
	}
	wg.Wait()
	return out
}

// ── MOBI ──────────────────────────────────────────────────────────────────────

type mobiRenderer struct{}

func (mobiRenderer) Ext() string  { return "mobi" }
func (mobiRenderer) Mime() string { return "application/x-mobipocket-ebook" }

func (mobiRenderer) Render(f Fetcher, req Request) ([]byte, string, error) {
	var htmlContent, title string
	if req.bulk() {
		title = firstNonEmpty(req.Title, "Articles")
		htmlContent = fetchAndCombine(f, req.URLs, title)
	} else {
		article, err := f.FetchReadable(req.URL)
		if err != nil {
			return nil, "", err
		}
		title = firstNonEmpty(req.Title, article.Title, "Article")
		htmlContent = mobiSingleBody(title, req.URL, article, f.FetchComments(req.CommentsURL))
	}

	var imageRecords [][]byte
	if req.EmbedImages {
		htmlContent, imageRecords = downloadAndEmbedMobiImages(htmlContent)
	}
	// Resolve TOC filepos links to byte offsets after image embedding has fixed
	// the final layout.
	htmlContent = patchMobiTOCFilepos(htmlContent)

	data, err := mobi.Write(mobi.Book{
		Title:   title,
		Author:  req.Author,
		Content: htmlContent,
		TOC:     buildMobiTOC(htmlContent),
	}, imageRecords)
	return data, title, err
}

// mobiSingleBody assembles a single-article MOBI document, including a
// heading-derived table of contents (with a Comments entry) when there are at
// least two navigation points.
func mobiSingleBody(title, url string, article readability.Article, commentsHTML string) string {
	link := `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>`
	annotated, labels := annotateArticleHeadings(article.Content, 0)
	hasComments := commentsHTML != ""
	total := len(labels)
	if hasComments {
		total++
	}

	var sb strings.Builder
	sb.WriteString("<html><body><h1>" + html.EscapeString(title) + "</h1>" + link + content.ArticleMeta(article))
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
	return sb.String()
}

// fetchAndCombine assembles a bulk MOBI document (filepos-based table of
// contents) from the concurrently fetched articles.
func fetchAndCombine(f Fetcher, urls []string, feedTitle string) string {
	results := acquireArticles(f, urls)

	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h1>" + html.EscapeString(feedTitle) + "</h1>")
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

// ── EPUB ──────────────────────────────────────────────────────────────────────

type epubRenderer struct{}

func (epubRenderer) Ext() string  { return "epub" }
func (epubRenderer) Mime() string { return "application/epub+zip" }

func (epubRenderer) Render(f Fetcher, req Request) ([]byte, string, error) {
	var body, title string
	if req.bulk() {
		title = firstNonEmpty(req.Title, "Articles")
		body = buildEpubMultiArticleBody(f, req.URLs, title)
	} else {
		article, err := f.FetchReadable(req.URL)
		if err != nil {
			return nil, "", err
		}
		title = firstNonEmpty(req.Title, article.Title, "Article")
		body = epubSingleBody(title, req.URL, article, f.FetchComments(req.CommentsURL))
	}
	data, err := generateEpub(title, req.Author, body, req.EmbedImages)
	return data, title, err
}

// epubSingleBody assembles a single-article EPUB body.
func epubSingleBody(title, url string, article readability.Article, commentsHTML string) string {
	link := `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>`
	body := "<h1>" + html.EscapeString(title) + "</h1>" + link + content.ArticleMeta(article) + article.Content
	if commentsHTML != "" {
		body += "<hr/><h2>Comments</h2>" + commentsHTML
	}
	return body
}

// buildEpubMultiArticleBody assembles a bulk EPUB body (anchor-based table of
// contents) from the concurrently fetched articles.
func buildEpubMultiArticleBody(f Fetcher, urls []string, feedTitle string) string {
	results := acquireArticles(f, urls)

	var sb strings.Builder
	sb.WriteString("<h1>" + html.EscapeString(feedTitle) + "</h1>")
	sb.WriteString("<h2>Contents</h2><ol>")
	for i, r := range results {
		if r.err != nil {
			sb.WriteString(fmt.Sprintf(`<li><a href="#article-%d">[Failed to fetch article]</a></li>`, i))
		} else {
			sb.WriteString(fmt.Sprintf(`<li><a href="#article-%d">%s</a></li>`, i, html.EscapeString(r.title)))
		}
	}
	sb.WriteString("</ol><hr/>")

	for i, r := range results {
		if r.err != nil {
			sb.WriteString(fmt.Sprintf(`<h2 id="article-%d">[Failed to fetch article]</h2><hr/>`, i))
		} else {
			sb.WriteString(fmt.Sprintf(`<h2 id="article-%d">%s</h2>`, i, html.EscapeString(r.title)))
			sb.WriteString(r.meta)
			sb.WriteString(r.content)
			sb.WriteString("<hr/>")
		}
	}
	return sb.String()
}
