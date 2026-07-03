package content

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type CommentsResponse struct {
	HTML string `json:"html"`
}

// Reddit JSON types
type redditListing struct {
	Data struct {
		Children []redditThing `json:"children"`
	} `json:"data"`
}

type redditThing struct {
	Kind string        `json:"kind"`
	Data redditComment `json:"data"`
}

type redditComment struct {
	Author     string          `json:"author"`
	CreatedUTC float64         `json:"created_utc"`
	BodyHTML   string          `json:"body_html"`
	Replies    json.RawMessage `json:"replies"` // string "" or listing object
}

// HN Algolia API types
type hnItem struct {
	ID        int      `json:"id"`
	Author    string   `json:"author"`
	CreatedAt string   `json:"created_at"`
	Text      string   `json:"text"`
	Children  []hnItem `json:"children"`
}

// Lobste.rs types
type lobstersStory struct {
	Comments []lobstersComment `json:"comments"`
}

type lobstersComment struct {
	ShortID        string          `json:"short_id"`
	CreatedAt      string          `json:"created_at"`
	IsDeleted      bool            `json:"is_deleted"`
	IsModerated    bool            `json:"is_moderated"`
	Comment        string          `json:"comment"`
	ParentComment  string          `json:"parent_comment"`
	CommentingUser json.RawMessage `json:"commenting_user"`
}

type lobstersNode struct {
	comment  lobstersComment
	children []*lobstersNode
}

// lobstersUsername extracts the username from the commenting_user field, which
// the Lobste.rs API returns as either {"username":"...",...} or a plain string.
func lobstersUsername(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "[deleted]"
	}
	if raw[0] == '{' {
		var u struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(raw, &u); err == nil && u.Username != "" {
			return u.Username
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s
	}
	return "[deleted]"
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTMLTags(s string) string {
	return htmlTagRe.ReplaceAllString(s, "")
}

const (
	maxTopLevelComments  = 100
	maxRepliesPerComment = 50
)

// Comments returns rendered comment HTML for rawURL, dispatching by source
// (Hacker News, Lobsters, Reddit .json, or a Readability fallback).
func (s *Service) Comments(rawURL string) (string, error) {
	switch {
	case strings.Contains(rawURL, "news.ycombinator.com/item?id="):
		return s.fetchHNComments(rawURL)
	case strings.Contains(rawURL, "lobste.rs/s/"):
		return s.fetchLobsteComments(rawURL)
	case strings.Contains(rawURL, ".json"):
		return s.fetchRedditComments(rawURL)
	default:
		article, err := s.FetchReadable(rawURL)
		if err != nil {
			return "", err
		}
		return article.Content, nil
	}
}

// FetchComments is the optional variant of Comments: it returns "" on any error
// (or empty URL) so callers can treat comments as best-effort.
func (s *Service) FetchComments(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	h, err := s.Comments(rawURL)
	if err != nil {
		return ""
	}
	return h
}

// ── HN ───────────────────────────────────────────────────────────────────────

func (s *Service) fetchHNComments(rawURL string) (string, error) {
	// Extract item ID from URL like https://news.ycombinator.com/item?id=12345
	idx := strings.Index(rawURL, "?id=")
	if idx < 0 {
		return "", fmt.Errorf("could not find item ID in HN URL")
	}
	itemID := rawURL[idx+4:]
	if i := strings.IndexAny(itemID, "&# "); i >= 0 {
		itemID = itemID[:i]
	}
	if itemID == "" {
		return "", fmt.Errorf("empty item ID in HN URL")
	}

	algoliaURL := "https://hn.algolia.com/api/v1/items/" + itemID
	client := s.newClient(ScrappingClientConfig{Timeout: 30 * time.Second, WithProxy: true, UseProxyFirst: true})
	hnReq, err := http.NewRequest("GET", algoliaURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(hnReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var item hnItem
	if err := json.Unmarshal(body, &item); err != nil {
		return "", fmt.Errorf("failed to parse HN JSON: %w", err)
	}

	if len(item.Children) == 0 {
		return "<p>No comments yet.</p>", nil
	}

	var sb strings.Builder
	counter := 0
	limit := len(item.Children)
	if limit > maxTopLevelComments {
		limit = maxTopLevelComments
	}
	for i := 0; i < limit; i++ {
		renderHNComment(&sb, item.Children[i], 0, &counter, i >= 10)
	}
	return sb.String(), nil
}

func renderHNComment(sb *strings.Builder, item hnItem, depth int, counter *int, collapsed bool) {
	n := *counter
	*counter++
	collapseID := fmt.Sprintf("hn-c-%d", n)
	author := item.Author
	if author == "" {
		author = "[deleted]"
	}

	dateStr := ""
	if item.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
			dateStr = t.Format("2006-01-02")
		} else {
			// fallback: take the date portion of the ISO string
			if len(item.CreatedAt) >= 10 {
				dateStr = item.CreatedAt[:10]
			}
		}
	}

	toggleIcon := "[&minus;]"
	bodyStyle := ""
	if collapsed {
		toggleIcon = "[+]"
		bodyStyle = ` style="display:none"`
	}

	sb.WriteString(`<div class="hn-comment">`)
	sb.WriteString(`<div class="hn-comment-header">`)
	fmt.Fprintf(sb, `<span id="%s-btn" class="hn-toggle" onclick="toggleHNComment('%s')">%s</span> `, collapseID, collapseID, toggleIcon)
	fmt.Fprintf(sb, `<strong class="hn-author">%s</strong>`, html.EscapeString(author))
	if dateStr != "" {
		fmt.Fprintf(sb, ` <span class="hn-date">%s</span>`, dateStr)
	}
	sb.WriteString(`</div>`)

	fmt.Fprintf(sb, `<div id="%s" class="hn-comment-body"%s>`, collapseID, bodyStyle)
	if item.Text != "" {
		fmt.Fprintf(sb, `<div class="hn-comment-text">%s</div>`, item.Text)
	} else {
		sb.WriteString(`<div class="hn-comment-text hn-deleted">[deleted]</div>`)
	}
	for _, child := range item.Children {
		renderHNComment(sb, child, depth+1, counter, true)
	}
	sb.WriteString(`</div>`) // hn-comment-body
	sb.WriteString(`</div>`) // hn-comment
}

// ── Reddit ───────────────────────────────────────────────────────────────────

func (s *Service) fetchRedditComments(rawURL string) (string, error) {
	client := s.newClient(ScrappingClientConfig{Timeout: 30 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Reddit returns [postListing, commentsListing]
	var listings []redditListing
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return "", fmt.Errorf("failed to parse Reddit JSON: %w", err)
	}
	if len(listings) < 2 {
		return "", fmt.Errorf("unexpected Reddit response format")
	}

	var sb strings.Builder
	counter := 0
	comments := listings[1].Data.Children
	limit := len(comments)
	if limit > maxTopLevelComments {
		limit = maxTopLevelComments
	}

	for i := 0; i < limit; i++ {
		replyCount := 0
		renderRedditComment(&sb, comments[i], 0, true, &replyCount, &counter, i >= 10)
	}

	if sb.Len() == 0 {
		return "<p>No comments found.</p>", nil
	}
	return sb.String(), nil
}

func renderRedditComment(sb *strings.Builder, thing redditThing, depth int, isTopLevel bool, replyCount *int, counter *int, collapsed bool) {
	if thing.Kind == "more" {
		return
	}
	if !isTopLevel {
		if *replyCount >= maxRepliesPerComment {
			return
		}
		(*replyCount)++
	}

	n := *counter
	*counter++
	collapseID := fmt.Sprintf("rc-%d", n)
	d := thing.Data
	author := d.Author
	if author == "" {
		author = "[deleted]"
	}

	toggleIcon := "[&minus;]"
	bodyStyle := ""
	if collapsed {
		toggleIcon = "[+]"
		bodyStyle = ` style="display:none"`
	}

	sb.WriteString(`<div class="hn-comment">`)
	sb.WriteString(`<div class="hn-comment-header">`)
	fmt.Fprintf(sb, `<span id="%s-btn" class="hn-toggle" onclick="toggleRedditComment('%s')">%s</span> `, collapseID, collapseID, toggleIcon)
	fmt.Fprintf(sb, `<strong class="hn-author">%s</strong>`, html.EscapeString(author))
	if d.CreatedUTC > 0 {
		t := time.Unix(int64(d.CreatedUTC), 0)
		fmt.Fprintf(sb, ` <span class="hn-date">%s</span>`, t.Format("2006-01-02"))
	}
	sb.WriteString(`</div>`)

	fmt.Fprintf(sb, `<div id="%s" class="hn-comment-body"%s>`, collapseID, bodyStyle)
	if d.BodyHTML != "" {
		decoded := html.UnescapeString(d.BodyHTML)
		text := stripHTMLTags(decoded)
		text = strings.ReplaceAll(text, "\n", "<br/>")
		fmt.Fprintf(sb, `<div class="hn-comment-text">%s</div>`, text)
	}

	// Recurse into replies inside the collapsible body
	if len(d.Replies) > 0 && d.Replies[0] == '{' {
		var repliesListing redditListing
		if err := json.Unmarshal(d.Replies, &repliesListing); err == nil {
			for _, child := range repliesListing.Data.Children {
				renderRedditComment(sb, child, depth+1, false, replyCount, counter, true)
			}
		}
	}

	sb.WriteString(`</div>`) // hn-comment-body
	sb.WriteString(`</div>`) // hn-comment
}

// ── Lobste.rs ─────────────────────────────────────────────────────────────────

func lobstersJSONURL(rawURL string) (string, error) {
	idx := strings.Index(rawURL, "/s/")
	if idx < 0 {
		return "", fmt.Errorf("could not find story short_id in Lobste.rs URL")
	}
	rest := rawURL[idx+3:]
	shortID := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		shortID = rest[:i]
	}
	if shortID == "" {
		return "", fmt.Errorf("empty story short_id in Lobste.rs URL")
	}
	return "https://lobste.rs/s/" + shortID + ".json", nil
}

func buildLobstersTree(comments []lobstersComment) []*lobstersNode {
	nodes := make(map[string]*lobstersNode, len(comments))
	for i := range comments {
		nodes[comments[i].ShortID] = &lobstersNode{comment: comments[i]}
	}
	var roots []*lobstersNode
	for i := range comments {
		node := nodes[comments[i].ShortID]
		if comments[i].ParentComment == "" {
			roots = append(roots, node)
		} else if parent, ok := nodes[comments[i].ParentComment]; ok {
			parent.children = append(parent.children, node)
		} else {
			roots = append(roots, node)
		}
	}
	return roots
}

func renderLobstersComment(sb *strings.Builder, node *lobstersNode, counter *int, collapsed bool) {
	c := node.comment
	n := *counter
	*counter++
	collapseID := fmt.Sprintf("lob-c-%d", n)

	author := "[deleted]"
	if !c.IsDeleted && !c.IsModerated {
		author = lobstersUsername(c.CommentingUser)
	}

	dateStr := ""
	if c.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
			dateStr = t.Format("2006-01-02")
		} else if len(c.CreatedAt) >= 10 {
			dateStr = c.CreatedAt[:10]
		}
	}

	toggleIcon := "[&minus;]"
	bodyStyle := ""
	if collapsed {
		toggleIcon = "[+]"
		bodyStyle = ` style="display:none"`
	}

	sb.WriteString(`<div class="hn-comment">`)
	sb.WriteString(`<div class="hn-comment-header">`)
	fmt.Fprintf(sb, `<span id="%s-btn" class="hn-toggle" onclick="toggleLobstersComment('%s')">%s</span> `, collapseID, collapseID, toggleIcon)
	fmt.Fprintf(sb, `<strong class="hn-author">%s</strong>`, html.EscapeString(author))
	if dateStr != "" {
		fmt.Fprintf(sb, ` <span class="hn-date">%s</span>`, dateStr)
	}
	sb.WriteString(`</div>`)

	fmt.Fprintf(sb, `<div id="%s" class="hn-comment-body"%s>`, collapseID, bodyStyle)
	if c.IsDeleted || c.IsModerated {
		sb.WriteString(`<div class="hn-comment-text hn-deleted">[deleted]</div>`)
	} else if c.Comment != "" {
		sb.WriteString(`<div class="hn-comment-text">`)
		sb.WriteString(c.Comment)
		sb.WriteString(`</div>`)
	}
	for _, child := range node.children {
		renderLobstersComment(sb, child, counter, true)
	}
	sb.WriteString(`</div>`) // hn-comment-body
	sb.WriteString(`</div>`) // hn-comment
}

func (s *Service) fetchLobsteComments(rawURL string) (string, error) {
	jsonURL, err := lobstersJSONURL(rawURL)
	if err != nil {
		return "", err
	}

	client := s.newClient(ScrappingClientConfig{Timeout: 30 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, err := http.NewRequest("GET", jsonURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var story lobstersStory
	if err := json.NewDecoder(resp.Body).Decode(&story); err != nil {
		return "", fmt.Errorf("failed to parse Lobste.rs JSON: %w", err)
	}

	if len(story.Comments) == 0 {
		return "<p>No comments yet.</p>", nil
	}

	roots := buildLobstersTree(story.Comments)
	var sb strings.Builder
	counter := 0
	limit := len(roots)
	if limit > maxTopLevelComments {
		limit = maxTopLevelComments
	}
	for i := 0; i < limit; i++ {
		renderLobstersComment(&sb, roots[i], &counter, i >= 10)
	}
	return sb.String(), nil
}
