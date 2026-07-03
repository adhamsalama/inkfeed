package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type GoogleNewsResponse struct {
	DecodedURL string `json:"decoded_url"`
}

// External Google News endpoints. Package vars so tests can redirect them to an
// httptest server. %s is substituted with the base64 article id.
var (
	googleNewsArticleURLs = []string{
		"https://news.google.com/articles/%s",
		"https://news.google.com/rss/articles/%s",
	}
	googleNewsBatchExecuteURL = "https://news.google.com/_/DotsSplashUi/data/batchexecute"
)

func decodeGoogleNewsHandler(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		jsonError(w, "url parameter required", http.StatusBadRequest)
		return
	}

	decoded, err := decodeGoogleNewsURL(rawURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GoogleNewsResponse{DecodedURL: decoded})
}

func decodeGoogleNewsURL(sourceURL string) (string, error) {
	base64Str, err := extractBase64(sourceURL)
	if err != nil {
		return "", err
	}

	sig, ts, err := getDecodingParams(base64Str)
	if err != nil {
		return "", err
	}

	return decodeViaAPI(base64Str, sig, ts)
}

func extractBase64(sourceURL string) (string, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Hostname() != "news.google.com" {
		return "", fmt.Errorf("not a Google News URL")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected Google News URL path")
	}
	secondLast := parts[len(parts)-2]
	if secondLast != "articles" && secondLast != "read" {
		return "", fmt.Errorf("unexpected Google News URL format")
	}
	return parts[len(parts)-1], nil
}

func getDecodingParams(base64Str string) (sig, ts string, err error) {
	client := &http.Client{Timeout: 15 * time.Second}

	urls := make([]string, len(googleNewsArticleURLs))
	for i, tmpl := range googleNewsArticleURLs {
		urls[i] = fmt.Sprintf(tmpl, base64Str)
	}

	for _, u := range urls {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")
		resp, rerr := client.Do(req)
		if rerr != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		sig, ts = extractDataAttrs(string(body))
		if sig != "" && ts != "" {
			return sig, ts, nil
		}
	}

	return "", "", fmt.Errorf("failed to fetch decoding parameters from Google News")
}

// extractDataAttrs parses HTML and finds data-n-a-sg / data-n-a-ts inside c-wiz > div[jscontroller].
func extractDataAttrs(htmlStr string) (sig, ts string) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return "", ""
	}

	var walk func(*html.Node) (string, string)
	walk = func(n *html.Node) (string, string) {
		if n.Type == html.ElementNode && n.Data == "c-wiz" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode {
					continue
				}
				var hasjscontroller, foundSig, foundTs bool
				var s, t string
				for _, a := range c.Attr {
					if a.Key == "jscontroller" {
						hasjscontroller = true
					}
					if a.Key == "data-n-a-sg" {
						s = a.Val
						foundSig = true
					}
					if a.Key == "data-n-a-ts" {
						t = a.Val
						foundTs = true
					}
				}
				if hasjscontroller && foundSig && foundTs {
					return s, t
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if s, t := walk(c); s != "" && t != "" {
				return s, t
			}
		}
		return "", ""
	}

	return walk(doc)
}

func decodeViaAPI(base64Str, sig, ts string) (string, error) {
	payload := []interface{}{
		[]interface{}{
			[]interface{}{
				"Fbv4je",
				fmt.Sprintf(`["garturlreq",[["X","X",["X","X"],null,null,1,1,"US:en",null,1,null,null,null,null,null,0,1],"X","X",1,[1,1,1],1,1,null,0,0,null,0],"%s",%s,"%s"]`,
					base64Str, ts, sig),
			},
		},
	}

	payloadJSON, _ := json.Marshal(payload)
	postData := "f.req=" + url.QueryEscape(string(payloadJSON))

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("POST", googleNewsBatchExecuteURL, strings.NewReader(postData))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("batchexecute request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Response is split by double newlines; the data is in the second chunk
	parts := strings.SplitN(bodyStr, "\n\n", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected batchexecute response format")
	}

	var outer []json.RawMessage
	if err := json.Unmarshal([]byte(parts[1]), &outer); err != nil || len(outer) < 1 {
		return "", fmt.Errorf("failed to parse batchexecute response")
	}

	// outer[0] is like ["wrb.fr","Fbv4je","<json-string>",...]
	var row []json.RawMessage
	if err := json.Unmarshal(outer[0], &row); err != nil || len(row) < 3 {
		return "", fmt.Errorf("unexpected batchexecute row structure")
	}

	var innerStr string
	if err := json.Unmarshal(row[2], &innerStr); err != nil {
		return "", fmt.Errorf("failed to parse inner JSON string: %w", err)
	}

	var inner []json.RawMessage
	if err := json.Unmarshal([]byte(innerStr), &inner); err != nil || len(inner) < 2 {
		return "", fmt.Errorf("failed to parse decoded URL response: %w", err)
	}

	var decodedURL string
	if err := json.Unmarshal(inner[1], &decodedURL); err != nil {
		return "", fmt.Errorf("failed to extract decoded URL: %w", err)
	}

	return decodedURL, nil
}
