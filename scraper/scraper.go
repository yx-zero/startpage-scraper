package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// single page
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	DisplayURL  string `json:"display_url"`
	Description string `json:"description"`
	SiteTitle   string `json:"site_title"`
	AnonViewURL string `json:"anon_view_url,omitempty"`
	Position    int    `json:"position"`
}

// packer
type SearchResults struct {
	Query        string            `json:"query"`
	TotalResults int               `json:"total_results"`
	Results      [][]*SearchResult `json:"results"`
}

func fetchSC(client *http.Client) (string, error) {
	req, _ := http.NewRequest("GET", "https://www.startpage.com/", nil)
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	sc := extractSC(string(body))
	if sc == "" {
		return "", fmt.Errorf("could not find sc token on homepage")
	}

	return sc, nil
}

// extractSC pulls the hidden "sc" session token out of a Startpage HTML page.
// Returns an empty string when the token is absent.
func extractSC(body string) string {
	const marker = `<input type="hidden" name="sc" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func FetchSearchQuery(query string, pages int, client *http.Client) (*SearchResults, error) {
	sc, err := fetchSC(client)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://www.startpage.com/sp/search", strings.NewReader(fmt.Sprintf(`query=%v&abp=1&abe=1&t=device&lui=english&sc=%v&cat=web&abd=1&abe=1`, query, sc)))
	if err != nil {
		return nil, err
	}
	setcommonheaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var sr []*SearchResult
	var newsc string

	// Startpage may serve an intermediate challenge page (containing a
	// "var data = {...}" blob) before returning results, or it may return the
	// results directly. Only run the challenge round-trip when the blob exists.
	if strings.Contains(string(bodyText), "var data =") {
		sr, newsc, err = handleExtraCaptcha(string(bodyText), query, client)
		if err != nil {
			return nil, err
		}
	} else {
		sr, err = parseStartpageHTML(string(bodyText))
		if err != nil {
			return nil, err
		}
		newsc = extractSC(string(bodyText))
	}

	srs := [][]*SearchResult{}
	srs = append(srs, sr)

	if pages == 1 {
		return &SearchResults{Query: query, TotalResults: countTotalResults(srs), Results: srs}, nil
	}

	for i := 2; i <= pages; i++ {
		srt, err := fetchResultPage(query, newsc, i, client)
		if err != nil {
			continue
		}

		srs = append(srs, srt)
	}

	return &SearchResults{Query: query, TotalResults: countTotalResults(srs), Results: srs}, nil
}

func handleExtraCaptcha(bodystr, query string, client *http.Client) ([]*SearchResult, string, error) {
	re := regexp.MustCompile(`var data = (\{[\s\S]*?\});`)
	matches := re.FindStringSubmatch(bodystr)
	if len(matches) < 2 {
		return nil, "", fmt.Errorf("no data found")
	}
	dataJSON := matches[1]

	var data map[string]string
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, "", err
	}

	data["query"] = query

	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	postBody := form.Encode()

	req, _ := http.NewRequest("POST", "https://www.startpage.com/sp/search", strings.NewReader(postBody))
	setcommonheaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	results, err := parseStartpageHTML(string(body))
	if err != nil {
		return nil, "", err
	}

	sc := extractSC(string(body))

	return results, sc, nil
}

func fetchResultPage(query, sc string, pagenum int, client *http.Client) ([]*SearchResult, error) {
	req, err := http.NewRequest("POST", "https://www.startpage.com/sp/search", strings.NewReader(fmt.Sprintf(`lui=english&language=english&query=%v&cat=web&sc=%v&t=device&segment=startpage.udog&page=%v&abd=1&abe=1`, query, sc, pagenum)))
	if err != nil {
		return nil, err
	}
	setcommonheaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseStartpageHTML(string(bodyText))
}

// parser!
func parseStartpageHTML(htmlContent string) ([]*SearchResult, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	results := make([]*SearchResult, 0)

	position := 1
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			if hasClass(n, "result") {
				result := extractResult(n, position)
				if result != nil {
					results = append(results, result)
					position++
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	return results, nil
}

func extractResult(n *html.Node, position int) *SearchResult {
	result := &SearchResult{
		Position: position,
	}

	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch {
			case node.Data == "h2" && hasClass(node, "wgl-title"):
				result.Title = cleanText(getTextContent(node))

			case node.Data == "a" && hasClass(node, "result-link"):
				if result.URL == "" {
					result.URL = getAttr(node, "href")
				}

			case node.Data == "a" && hasClass(node, "wgl-display-url"):
				if result.DisplayURL == "" {
					result.DisplayURL = extractDisplayURL(node)
				}

			case node.Data == "p" && hasClass(node, "description"):
				result.Description = cleanText(getTextContent(node))

			case node.Data == "a" && hasClass(node, "wgl-site-title"):
				result.SiteTitle = cleanText(getTextContent(node))

			case hasClass(node, "anonymous-view-link"):
				anonLink := findAnchorInNode(node)
				if anonLink != "" && result.AnonViewURL == "" {
					result.AnonViewURL = anonLink
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(n)

	if result.Title != "" && result.URL != "" {
		return result
	}
	return nil
}

func extractDisplayURL(n *html.Node) string {
	var structuredText, defaultText string

	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "style" {
			return
		}

		if node.Type == html.ElementNode && node.Data == "span" {
			if hasClass(node, "structured-link-text") {
				structuredText = cleanText(getTextContent(node))
			} else if hasClass(node, "default-link-text") {
				defaultText = cleanText(getTextContent(node))
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(n)

	if structuredText != "" {
		return structuredText
	}
	if defaultText != "" {
		return defaultText
	}

	return getAttr(n, "href")
}

func findAnchorInNode(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "a" {
		return getAttr(n, "href")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := findAnchorInNode(c); href != "" {
			return href
		}
	}
	return ""
}

// helper functions

func hasClass(n *html.Node, className string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			classes := strings.Fields(attr.Val)
			for _, c := range classes {
				if c == className {
					return true
				}
			}
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func getTextContent(n *html.Node) string {
	var text strings.Builder
	var traverse func(*html.Node)
	traverse = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(n)
	return text.String()
}

func cleanText(text string) string {

	text = strings.TrimSpace(text)

	text = strings.Join(strings.Fields(text), " ")

	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&middot;", "·")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&#x27;", "'")
	text = strings.ReplaceAll(text, "&quot;", "\"")

	text = strings.ReplaceAll(text, "<b>", "")
	text = strings.ReplaceAll(text, "</b>", "")
	return text
}

func (sr *SearchResults) ToJSON() (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(sr)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func countTotalResults(r [][]*SearchResult) int {
	totalCount := 0
	for _, page := range r {
		totalCount += len(page)
	}
	return totalCount
}

func setcommonheaders(req *http.Request) {
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "en-US,en;q=0.9")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("origin", "https://www.startpage.com")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("priority", "u=0, i")
	req.Header.Set("referer", "https://www.startpage.com/")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
}
