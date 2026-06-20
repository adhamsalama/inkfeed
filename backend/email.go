package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"time"

	"github.com/adhamsalama/inkfeed-backend/mobi"
)

// EmailAttachment is a file to attach to an email.
type EmailAttachment struct {
	Filename string
	Content  []byte
	MimeType string
}

// EmailMessage is the provider-agnostic email to send.
type EmailMessage struct {
	To          string
	Subject     string
	HTMLContent string
	Attachments []EmailAttachment
}

// EmailSender is the interface for sending emails.
// Swap implementations by changing newEmailSender().
type EmailSender interface {
	Send(msg EmailMessage) error
}

// newEmailSender returns the configured EmailSender based on EMAIL_PROVIDER env var.
// Supported values: "brevo" (default), "mailersend", "resend".
func newEmailSender() EmailSender {
	apiKey := os.Getenv("EMAIL_API_KEY")
	fromEmail := os.Getenv("EMAIL_FROM")
	fromName := os.Getenv("EMAIL_FROM_NAME")
	switch os.Getenv("EMAIL_PROVIDER") {
	case "mailersend":
		return &MailerSendSender{APIKey: apiKey, FromEmail: fromEmail, FromName: fromName}
	case "resend":
		return &ResendSender{APIKey: apiKey, FromEmail: fromEmail, FromName: fromName}
	default:
		return &BrevoSender{APIKey: apiKey, FromEmail: fromEmail, FromName: fromName}
	}
}

// BrevoSender sends emails via the Brevo (Sendinblue) transactional API.
type BrevoSender struct {
	APIKey    string
	FromEmail string
	FromName  string
}

func (b *BrevoSender) Send(msg EmailMessage) error {
	type contact struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}
	type attachment struct {
		Name    string `json:"name"`
		Content string `json:"content"` // base64-encoded
	}
	type payload struct {
		Sender      contact      `json:"sender"`
		To          []contact    `json:"to"`
		Subject     string       `json:"subject"`
		HTMLContent string       `json:"htmlContent"`
		Attachment  []attachment `json:"attachment,omitempty"`
	}

	p := payload{
		Sender:      contact{Email: b.FromEmail, Name: b.FromName},
		To:          []contact{{Email: msg.To}},
		Subject:     msg.Subject,
		HTMLContent: msg.HTMLContent,
	}
	for _, a := range msg.Attachments {
		p.Attachment = append(p.Attachment, attachment{
			Name:    a.Filename,
			Content: base64.StdEncoding.EncodeToString(a.Content),
		})
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal brevo request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", "https://api.brevo.com/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	// req.Header.Set("api-key", b.APIKey)
	req.Header.Set("Authorization", "Bearer "+b.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("brevo API error %d: %v", resp.StatusCode, errBody)
	}
	return nil
}

// MailerSendSender sends emails via the MailerSend transactional API.
type MailerSendSender struct {
	APIKey    string
	FromEmail string
	FromName  string
}

func (m *MailerSendSender) Send(msg EmailMessage) error {
	type contact struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}
	type attachment struct {
		Filename string `json:"filename"`
		Content  string `json:"content"` // base64-encoded
	}
	type payload struct {
		From        contact      `json:"from"`
		To          []contact    `json:"to"`
		Subject     string       `json:"subject"`
		HTML        string       `json:"html"`
		Attachments []attachment `json:"attachments,omitempty"`
	}

	p := payload{
		From:    contact{Email: m.FromEmail, Name: m.FromName},
		To:      []contact{{Email: msg.To}},
		Subject: msg.Subject,
		HTML:    msg.HTMLContent,
	}
	for _, a := range msg.Attachments {
		p.Attachments = append(p.Attachments, attachment{
			Filename: a.Filename,
			Content:  base64.StdEncoding.EncodeToString(a.Content),
		})
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal mailersend request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", "https://api.mailersend.com/v1/email", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mailersend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("mailersend API error %d: %v", resp.StatusCode, errBody)
	}
	return nil
}

// ResendSender sends emails via the Resend transactional API.
type ResendSender struct {
	APIKey    string
	FromEmail string
	FromName  string
}

func (r *ResendSender) Send(msg EmailMessage) error {
	from := r.FromEmail
	if r.FromName != "" {
		from = r.FromName + " <" + r.FromEmail + ">"
	}
	type attachment struct {
		Filename string `json:"filename"`
		Content  string `json:"content"` // base64-encoded
	}
	type payload struct {
		From        string       `json:"from"`
		To          []string     `json:"to"`
		Subject     string       `json:"subject"`
		HTML        string       `json:"html"`
		Attachments []attachment `json:"attachments,omitempty"`
	}

	p := payload{
		From:    from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTMLContent,
	}
	for _, a := range msg.Attachments {
		p.Attachments = append(p.Attachments, attachment{
			Filename: a.Filename,
			Content:  base64.StdEncoding.EncodeToString(a.Content),
		})
	}

	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal resend request: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("resend API error %d: %v", resp.StatusCode, errBody)
	}
	return nil
}

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

func emailHandler(w http.ResponseWriter, r *http.Request) {
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
	if req.Format == "" {
		req.Format = "epub"
	}

	// Bulk email
	if len(req.URLs) > 0 {
		title := req.Author
		if title == "" {
			title = "Articles"
		}
		var msg EmailMessage
		msg.To = req.To
		msg.Subject = "Your exported articles are ready"
		switch req.Format {
		case "mobi":
			htmlContent := fetchAndCombine(req.URLs, title)
			embedImagesMobi := req.EmbedImages == nil || *req.EmbedImages
			var mobiImgRecords [][]byte
			if embedImagesMobi {
				htmlContent, mobiImgRecords = downloadAndEmbedMobiImages(htmlContent)
			}
			data, err := mobi.Write(mobi.Book{Title: title, Author: req.Author, Content: htmlContent}, mobiImgRecords)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			msg.HTMLContent = "<p>" + html.EscapeString(title) + "</p>"
			msg.Attachments = []EmailAttachment{{
				Filename: sanitizeFilename(title) + "_" + time.Now().Format("2006-01-02") + ".mobi",
				Content:  data,
				MimeType: "application/x-mobipocket-ebook",
			}}
		default: // epub
			xhtmlBody := buildEpubMultiArticleBody(req.URLs, title)
			data, err := generateEpub(title, req.Author, xhtmlBody, req.EmbedImages == nil || *req.EmbedImages)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			msg.HTMLContent = "<p>" + html.EscapeString(title) + "</p>"
			msg.Attachments = []EmailAttachment{{
				Filename: sanitizeFilename(title) + "_" + time.Now().Format("2006-01-02") + ".epub",
				Content:  data,
				MimeType: "application/epub+zip",
			}}
		}
		sender := newEmailSender()
		if err := sender.Send(msg); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	article, err := fetchReadable(req.URL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	title := article.Title
	if title == "" {
		title = "Article"
	}

	links := `<p><a href="` + html.EscapeString(req.URL) + `">` + html.EscapeString(req.URL) + `</a></p>`
	if req.CommentsURL != "" {
		links += `<p><a href="` + html.EscapeString(req.CommentsURL) + `">Comments</a></p>`
	}
	commentsHTML := fetchCommentsHTML(req.CommentsURL)
	meta := articleMetaHTML(article)
	articleHTML := "<html><body><h1>" + html.EscapeString(title) + "</h1>" + links + meta + article.Content
	if commentsHTML != "" {
		articleHTML += "<hr/><h2>Comments</h2>" + commentsHTML
	}
	articleHTML += "</body></html>"

	var msg EmailMessage
	msg.To = req.To
	msg.Subject = "Your exported article is ready"

	switch req.Format {
	case "mobi":
		embedImagesMobi := req.EmbedImages == nil || *req.EmbedImages
		var mobiImgRecords [][]byte
		if embedImagesMobi {
			articleHTML, mobiImgRecords = downloadAndEmbedMobiImages(articleHTML)
		}
		data, err := mobi.Write(mobi.Book{Title: title, Author: req.Author, Content: articleHTML}, mobiImgRecords)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msg.HTMLContent = "<p>" + html.EscapeString(title) + "</p>"
		msg.Attachments = []EmailAttachment{{
			Filename: sanitizeFilename(title) + ".mobi",
			Content:  data,
			MimeType: "application/x-mobipocket-ebook",
		}}
	default: // "epub"
		embedImages := req.EmbedImages == nil || *req.EmbedImages
		xhtmlBody := "<h1>" + html.EscapeString(title) + "</h1>" + links + meta + article.Content
		if commentsHTML != "" {
			xhtmlBody += "<hr/><h2>Comments</h2>" + commentsHTML
		}
		data, err := generateEpub(title, req.Author, xhtmlBody, embedImages)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msg.HTMLContent = "<p>" + html.EscapeString(title) + "</p>"
		msg.Attachments = []EmailAttachment{{
			Filename: sanitizeFilename(title) + ".epub",
			Content:  data,
			MimeType: "application/epub+zip",
		}}
	}

	sender := newEmailSender()
	if err := sender.Send(msg); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

