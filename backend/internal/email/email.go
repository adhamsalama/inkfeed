// Package email sends transactional email with file attachments through a
// pluggable provider (Brevo, MailerSend, or Resend), selected at runtime via the
// EMAIL_PROVIDER environment variable.
package email

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// Attachment is a file to attach to an email.
type Attachment struct {
	Filename string
	Content  []byte
	MimeType string
}

// Message is the provider-agnostic email to send.
type Message struct {
	To          string
	Subject     string
	HTMLContent string
	Attachments []Attachment
}

// Sender sends an email. Swap implementations via NewSender.
type Sender interface {
	Send(msg Message) error
}

// NewSender returns the configured Sender based on the EMAIL_PROVIDER env var.
// Supported values: "brevo" (default), "mailersend", "resend".
func NewSender() Sender {
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

func (b *BrevoSender) Send(msg Message) error {
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
	req.Header.Set("Authorization", "Bearer "+b.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("brevo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var errBody map[string]any
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

func (m *MailerSendSender) Send(msg Message) error {
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
		var errBody map[string]any
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

func (r *ResendSender) Send(msg Message) error {
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
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("resend API error %d: %v", resp.StatusCode, errBody)
	}
	return nil
}
