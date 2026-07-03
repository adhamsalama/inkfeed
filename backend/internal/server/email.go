package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"time"
)

// External provider endpoints. Declared as package vars so tests can point them
// at an httptest server instead of hitting the live APIs.
var (
	brevoAPIURL      = "https://api.brevo.com/v3/smtp/email"
	mailerSendAPIURL = "https://api.mailersend.com/v1/email"
	resendAPIURL     = "https://api.resend.com/emails"
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
	req, err := http.NewRequest("POST", brevoAPIURL, bytes.NewReader(body))
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
	req, err := http.NewRequest("POST", mailerSendAPIURL, bytes.NewReader(body))
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
	req, err := http.NewRequest("POST", resendAPIURL, bytes.NewReader(body))
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
	er := exportRequest{
		url:         req.URL,
		urls:        req.URLs,
		commentsURL: req.CommentsURL,
		author:      req.Author,
		embedImages: req.EmbedImages == nil || *req.EmbedImages,
	}
	subject := "Your exported article is ready"
	if er.bulk() {
		er.title = firstNonEmpty(req.Author, "Articles")
		subject = "Your exported articles are ready"
	}

	rd := rendererForFormat(req.Format)
	data, title, err := rd.render(a, er)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	msg := EmailMessage{
		To:          req.To,
		Subject:     subject,
		HTMLContent: "<p>" + html.EscapeString(title) + "</p>",
		Attachments: []EmailAttachment{{
			Filename: exportFilename(title, rd.ext(), er.bulk()),
			Content:  data,
			MimeType: rd.mime(),
		}},
	}

	if err := newEmailSender().Send(msg); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
