package main

import (
	"encoding/json"
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

func redditPostHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}

	client := newScrapingClient(ScrapingClientConfig{Timeout: 15 * time.Second})
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Reddit JSON is a 2-element array: [post_listing, comments_listing]
	var listings []json.RawMessage
	if err := json.Unmarshal(body, &listings); err != nil || len(listings) == 0 {
		jsonError(w, "failed to parse Reddit JSON", http.StatusBadGateway)
		return
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
		jsonError(w, "unexpected Reddit JSON structure", http.StatusBadGateway)
		return
	}

	postData := postListing.Data.Children[0].Data

	// Determine the actual article URL
	actualURL := ""
	if !postData.IsSelf && postData.URL != "" {
		actualURL = postData.URL
	}

	// Build content HTML from selftext_html or selftext
	contentHTML := ""
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
		contentHTML = decoded
	} else if postData.Selftext != "" {
		contentHTML = "<p>" + html.EscapeString(postData.Selftext) + "</p>"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RedditPostResponse{
		ActualURL:   actualURL,
		ContentHTML: contentHTML,
	})
}
