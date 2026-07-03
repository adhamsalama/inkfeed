package server

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/adhamsalama/inkfeed-backend/mobi"
	readability "github.com/go-shiori/go-readability"
)

// fetchedArticle is the raw material produced by acquireArticles and consumed by
// the format renderers.
type fetchedArticle struct {
	url     string
	title   string
	meta    string
	content string // link line + readable HTML
	err     error
}

// acquireArticles fetches every URL concurrently (max 5 in flight), preserving
// input order. It is the single content-acquisition path shared by every bulk
// exporter (MOBI, EPUB, email) — replacing what used to be two near-identical
// goroutine loops.
func (a *App) acquireArticles(urls []string) []fetchedArticle {
	out := make([]fetchedArticle, len(urls))
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
				out[idx] = fetchedArticle{url: url, err: err}
				return
			}
			out[idx] = fetchedArticle{
				url:     url,
				title:   article.Title,
				meta:    articleMetaHTML(article),
				content: `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>` + article.Content,
			}
		}(i, u)
	}
	wg.Wait()
	return out
}

// exportRequest is the format-agnostic description of an export job. Both a
// single article (url + optional commentsURL) and a bulk export (urls) flow
// through the same shape.
type exportRequest struct {
	url         string
	urls        []string
	commentsURL string
	title       string
	author      string
	embedImages bool
}

func (req exportRequest) bulk() bool { return len(req.urls) > 0 }

// Renderer turns an export request into a downloadable document. mobiRenderer and
// epubRenderer are the two implementations; a new output format means one more
// Renderer, not another branch in three handlers.
type Renderer interface {
	render(a *App, req exportRequest) (data []byte, title string, err error)
	ext() string
	mime() string
}

// rendererForFormat maps the request "format" field to a Renderer (epub default).
func rendererForFormat(format string) Renderer {
	if format == "mobi" {
		return mobiRenderer{}
	}
	return epubRenderer{}
}

// exportFilename builds the download filename: "<title>.<ext>" for a single
// article, "<title>_<date>.<ext>" for a bulk export.
func exportFilename(title, ext string, bulk bool) string {
	name := sanitizeFilename(title)
	if bulk {
		return name + "_" + time.Now().Format("2006-01-02") + "." + ext
	}
	return name + "." + ext
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── MOBI ──────────────────────────────────────────────────────────────────────

type mobiRenderer struct{}

func (mobiRenderer) ext() string  { return "mobi" }
func (mobiRenderer) mime() string { return "application/x-mobipocket-ebook" }

func (mobiRenderer) render(a *App, req exportRequest) ([]byte, string, error) {
	var htmlContent, title string
	if req.bulk() {
		title = firstNonEmpty(req.title, "Articles")
		htmlContent = a.fetchAndCombine(req.urls, title)
	} else {
		article, err := a.fetchReadable(req.url)
		if err != nil {
			return nil, "", err
		}
		title = firstNonEmpty(req.title, article.Title, "Article")
		htmlContent = a.mobiSingleBody(title, req.url, article, a.fetchCommentsHTML(req.commentsURL))
	}

	var imageRecords [][]byte
	if req.embedImages {
		htmlContent, imageRecords = downloadAndEmbedMobiImages(htmlContent)
	}
	// Resolve TOC filepos links to byte offsets after image embedding has fixed
	// the final layout.
	htmlContent = patchMobiTOCFilepos(htmlContent)

	data, err := mobi.Write(mobi.Book{
		Title:   title,
		Author:  req.author,
		Content: htmlContent,
		TOC:     buildMobiTOC(htmlContent),
	}, imageRecords)
	return data, title, err
}

// mobiSingleBody assembles a single-article MOBI document, including a
// heading-derived table of contents (with a Comments entry) when there are at
// least two navigation points.
func (a *App) mobiSingleBody(title, url string, article readability.Article, commentsHTML string) string {
	link := `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>`
	annotated, labels := annotateArticleHeadings(article.Content, 0)
	hasComments := commentsHTML != ""
	total := len(labels)
	if hasComments {
		total++
	}

	var sb strings.Builder
	sb.WriteString("<html><body><h1>" + html.EscapeString(title) + "</h1>" + link + articleMetaHTML(article))
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
func (a *App) fetchAndCombine(urls []string, feedTitle string) string {
	results := a.acquireArticles(urls)

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

func (epubRenderer) ext() string  { return "epub" }
func (epubRenderer) mime() string { return "application/epub+zip" }

func (epubRenderer) render(a *App, req exportRequest) ([]byte, string, error) {
	var body, title string
	if req.bulk() {
		title = firstNonEmpty(req.title, "Articles")
		body = a.buildEpubMultiArticleBody(req.urls, title)
	} else {
		article, err := a.fetchReadable(req.url)
		if err != nil {
			return nil, "", err
		}
		title = firstNonEmpty(req.title, article.Title, "Article")
		body = a.epubSingleBody(title, req.url, article, a.fetchCommentsHTML(req.commentsURL))
	}
	data, err := generateEpub(title, req.author, body, req.embedImages)
	return data, title, err
}

// epubSingleBody assembles a single-article EPUB body.
func (a *App) epubSingleBody(title, url string, article readability.Article, commentsHTML string) string {
	link := `<p><a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + `</a></p>`
	body := "<h1>" + html.EscapeString(title) + "</h1>" + link + articleMetaHTML(article) + article.Content
	if commentsHTML != "" {
		body += "<hr/><h2>Comments</h2>" + commentsHTML
	}
	return body
}

// buildEpubMultiArticleBody assembles a bulk EPUB body (anchor-based table of
// contents) from the concurrently fetched articles.
func (a *App) buildEpubMultiArticleBody(urls []string, feedTitle string) string {
	results := a.acquireArticles(urls)

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
