package server

import (
	"encoding/json"
	"net/http"

	"github.com/adhamsalama/inkfeed-backend/internal/export"
)

// exportRequestBody is the JSON body for the /mobi and /epub endpoints.
type exportRequestBody struct {
	URL         string   `json:"url"`
	URLs        []string `json:"urls"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"`
	EmbedImages *bool    `json:"embedImages"` // default true
}

func (b exportRequestBody) toExport() export.Request {
	return export.Request{
		URL:         b.URL,
		URLs:        b.URLs,
		CommentsURL: b.CommentsURL,
		Title:       b.Title,
		Author:      b.Author,
		EmbedImages: b.EmbedImages == nil || *b.EmbedImages,
	}
}

// handleExport is the shared body for the /mobi and /epub download handlers.
func (a *App) handleExport(w http.ResponseWriter, r *http.Request, rd export.Renderer) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body exportRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.URL == "" && len(body.URLs) == 0 {
		jsonError(w, "url or urls field required", http.StatusBadRequest)
		return
	}
	req := body.toExport()
	data, title, err := rd.Render(a.content, req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", rd.Mime())
	w.Header().Set("Content-Disposition", `attachment; filename="`+export.FilenameForRequest(req, title, rd.Ext())+`"`)
	w.Write(data)
}

func (a *App) mobiHandler(w http.ResponseWriter, r *http.Request) {
	a.handleExport(w, r, export.RendererFor("mobi"))
}

func (a *App) epubHandler(w http.ResponseWriter, r *http.Request) {
	a.handleExport(w, r, export.RendererFor("epub"))
}
