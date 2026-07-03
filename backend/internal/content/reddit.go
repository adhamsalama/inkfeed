package content

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

type RedditPostResponse struct {
	ActualURL   string `json:"actual_url"`
	ContentHTML string `json:"content_html"`
}

// RedditPost fetches a Reddit post's JSON and extracts its self-text HTML (or the
// linked article URL for link posts).
func (s *Service) RedditPost(rawURL string) (RedditPostResponse, error) {
	var out RedditPostResponse

	client := s.newClient(ScrappingClientConfig{Timeout: 15 * time.Second, WithProxy: true, UseProxyFirst: true})
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return out, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, err
	}

	// Reddit JSON is a 2-element array: [post_listing, comments_listing]
	var listings []json.RawMessage
	if err := json.Unmarshal(body, &listings); err != nil || len(listings) == 0 {
		return out, fmt.Errorf("failed to parse Reddit JSON")
	}

	var postListing struct {
		Data struct {
			Children []struct {
				Data struct {
					IsSelf       bool   `json:"is_self"`
					URL          string `json:"url"`
					Selftext     string `json:"selftext"`
					SelftextHTML string `json:"selftext_html"`
					Permalink    string `json:"permalink"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listings[0], &postListing); err != nil || len(postListing.Data.Children) == 0 {
		return out, fmt.Errorf("unexpected Reddit JSON structure")
	}

	postData := postListing.Data.Children[0].Data

	// Determine the actual article URL
	if !postData.IsSelf && postData.URL != "" {
		out.ActualURL = postData.URL
	}

	// Build content HTML from selftext_html or selftext
	if postData.SelftextHTML != "" {
		// selftext_html is HTML-entity-encoded; decode it
		decoded := html.UnescapeString(postData.SelftextHTML)
		// Strip SC_OFF/SC_ON comments and outer <div class="md"> wrapper
		decoded = strings.ReplaceAll(decoded, "<!-- SC_OFF -->", "")
		decoded = strings.ReplaceAll(decoded, "<!-- SC_ON -->", "")
		decoded = strings.TrimSpace(decoded)
		if strings.HasPrefix(decoded, `<div class="md">`) && strings.HasSuffix(decoded, "</div>") {
			decoded = decoded[len(`<div class="md">`) : len(decoded)-len("</div>")]
		}
		out.ContentHTML = decoded
	} else if postData.Selftext != "" {
		out.ContentHTML = "<p>" + html.EscapeString(postData.Selftext) + "</p>"
	}

	return out, nil
}
