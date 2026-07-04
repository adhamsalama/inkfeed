package server

import (
	"encoding/json"
	"html"
	"net/http"

	"github.com/adhamsalama/inkfeed-backend/internal/email"
	"github.com/adhamsalama/inkfeed-backend/internal/export"
)

// EmailRequest is the request body for POST /email.
// Format is "epub" or "mobi" (defaults to "epub").
type EmailRequest struct {
	URL         string   `json:"url"`
	URLs        []string `json:"urls"`
	To          string   `json:"to"`
	Format      string   `json:"format"`
	Author      string   `json:"author"`
	CommentsURL string   `json:"commentsUrl"`
	EmbedImages *bool    `json:"embedImages"`
}

func (a *App) emailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if (req.URL == "" && len(req.URLs) == 0) || req.To == "" {
		jsonError(w, "url or urls and to fields required", http.StatusBadRequest)
		return
	}

	// The same Renderer that powers the /mobi and /epub download endpoints
	// produces the attachment here — no per-format or single/bulk branching.
	bulk := len(req.URLs) > 0
	er := export.Request{
		URL:         req.URL,
		URLs:        req.URLs,
		CommentsURL: req.CommentsURL,
		Author:      req.Author,
		EmbedImages: req.EmbedImages == nil || *req.EmbedImages,
	}
	subject := "Your exported article is ready"
	if bulk {
		er.Title = req.Author
		if er.Title == "" {
			er.Title = "Articles"
		}
		subject = "Your exported articles are ready"
	}

	rd := export.RendererFor(req.Format)
	data, title, err := rd.Render(a.content, er)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	msg := email.Message{
		To:          req.To,
		Subject:     subject,
		HTMLContent: "<p>" + html.EscapeString(title) + "</p>",
		Attachments: []email.Attachment{{
			Filename: export.FilenameForRequest(er, title, rd.Ext()),
			Content:  data,
			MimeType: rd.Mime(),
		}},
	}

	if err := a.sender.Send(msg); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
