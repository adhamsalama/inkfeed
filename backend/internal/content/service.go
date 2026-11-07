// Package content acquires content from the web: RSS/Atom feed lists, readable
// single-article extraction, and comments (Hacker News, Reddit, Lobsters, and a
// Readability fallback). It also runs the background feed scraper and article
// archiver. All outbound HTTP goes through an internal proxy-aware client.
package content

import "github.com/adhamsalama/inkfeed-backend/db"

// UserAgent is sent on every outbound scraping/image request.
const UserAgent = "Inkfeed/1.0 (+https://inkfeed.xyz)"

// Service is the content-acquisition capability. ProxyURL is the (optional)
// CORS/scraping proxy base URL; q is the persistence layer for the archive.
type Service struct {
	ProxyURL string
	q        *db.Queries
}

// New builds a Service backed by the given queries. Callers set ProxyURL after
// construction (it comes from configuration).
func New(q *db.Queries) *Service {
	return &Service{q: q}
}
