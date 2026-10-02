package content

import (
	"context"
	"html"
	"strings"
	"unicode"

	"github.com/adhamsalama/inkfeed-backend/db"
)

const (
	snippetLeadRunes = 60
	snippetRunes     = 180
)

// FTSQuery turns free-form user input into a safe FTS5 query: every
// whitespace-separated word becomes a quoted term, and all terms must match.
// Quoting neutralises FTS5 operators (AND, NOT, *, :, -, parentheses) so user
// input can never cause a syntax error. Returns "" if the input has no words.
func FTSQuery(raw string) string {
	var terms []string
	for _, word := range strings.Fields(raw) {
		word = strings.ReplaceAll(word, `"`, "")
		if word != "" {
			terms = append(terms, `"`+word+`"`)
		}
	}
	return strings.Join(terms, " ")
}

// SearchFeedArchive full-text searches the archived articles of a feed. Each
// result's Description is an HTML-escaped excerpt of the article text around
// the first match, falling back to the feed item's own description.
func (s *Service) SearchFeedArchive(feedURL, query string, limit, offset int64) (FeedArchivePage, error) {
	ctx := context.Background()
	match := FTSQuery(query)
	if match == "" {
		return FeedArchivePage{Articles: []ArchiveArticle{}}, nil
	}

	rows, err := s.q.SearchFeedArchive(ctx, db.SearchFeedArchiveParams{
		Doc:     match,
		FeedUrl: feedURL,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return FeedArchivePage{}, err
	}

	total, err := s.q.CountSearchFeedArchive(ctx, db.CountSearchFeedArchiveParams{Doc: match, FeedUrl: feedURL})
	if err != nil {
		total = 0
	}

	terms := strings.Fields(strings.ReplaceAll(query, `"`, ""))
	articles := make([]ArchiveArticle, len(rows))
	for i, row := range rows {
		desc := row.Description
		if snip := matchSnippet(row.TextContent, terms); snip != "" {
			desc = html.EscapeString(snip)
		}
		articles[i] = ArchiveArticle{
			Index:       int(offset) + i,
			Title:       row.Title,
			Link:        row.ItemUrl,
			Description: desc,
			PubDate:     row.PubDate,
			Comments:    row.CommentsUrl.String,
		}
	}

	return FeedArchivePage{
		Articles: articles,
		Total:    total,
		HasMore:  offset+int64(len(rows)) < total,
	}, nil
}

// matchSnippet returns a short excerpt of text starting a little before the
// earliest occurrence of any term (case-insensitive), or "" if none occur.
// Because the index stems words, a term that doesn't appear verbatim is retried
// without its last two letters as a rough stand-in ("searches" → "search").
func matchSnippet(text string, terms []string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}

	pos := -1
	for _, term := range terms {
		t := []rune(strings.ToLower(term))
		idx := indexRunes(lower, t)
		if idx < 0 && len(t) > 4 {
			idx = indexRunes(lower, t[:len(t)-2])
		}
		if idx >= 0 && (pos < 0 || idx < pos) {
			pos = idx
		}
	}
	if pos < 0 {
		return ""
	}

	start := pos - snippetLeadRunes
	if start <= 0 {
		start = 0
	} else {
		// Start on a word boundary.
		for start < pos && runes[start-1] != ' ' {
			start++
		}
	}
	end := start + snippetRunes
	if end > len(runes) {
		end = len(runes)
	}

	snip := string(runes[start:end])
	if start > 0 {
		snip = "…" + snip
	}
	if end < len(runes) {
		snip += "…"
	}
	return snip
}

func indexRunes(s, sub []rune) int {
	if len(sub) == 0 {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := range sub {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
