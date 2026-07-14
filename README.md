<h1 align="center">startpage-scraper</h1>

<p align="center">
  <strong>
    A Go scraper for <a href="https://www.startpage.com/">Startpage</a>
  </strong>
</p>

<p align="center">
  Programmatically scrape search results from Startpage with support for pagination and JSON output.
</p>

---

## Features

- Scrapes Startpage web search results
- Supports multiple result pages
- Automatically handles session tokens and (when present) challenge pages
- Extracts:
    - Title
    - URL
    - Display URL
    - Description
    - Site title
    - Anonymous view URL (when available)
- Clean, structured output
- JSON export
- Works with any `*http.Client`, so proxies, timeouts, and cookie jars are fully under your control

---

## Installation

```bash
go get github.com/yx-zero/startpage-scraper
```

---

## Usage

Pass in your own `*http.Client`. A cookie jar is **required**: Startpage ties the
session token (`sc`) and its cookies together across the homepage → search →
pagination requests.

> **Note:** URL-encode your query before passing it in. The library interpolates
> the query string directly into the request body.

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/yx-zero/startpage-scraper/scraper"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}

	query := url.QueryEscape("best programming language in 2026")
	pages := 2

	results, err := scraper.FetchSearchQuery(query, pages, client)
	if err != nil {
		log.Fatal(err)
	}

	out, err := results.ToJSON()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
}
```

### Using a proxy

Because the scraper uses the `*http.Client` you supply, routing traffic through a
proxy is just a matter of configuring the client's `Transport`:

```go
proxyURL, _ := url.Parse("http://user:pass@host:8080") // or socks5://host:1080
jar, _ := cookiejar.New(nil)

client := &http.Client{
	Jar:       jar,
	Timeout:   30 * time.Second,
	Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
}

results, err := scraper.FetchSearchQuery(url.QueryEscape("golang concurrency"), 1, client)
```

To honor the `HTTP_PROXY` / `HTTPS_PROXY` environment variables instead, either
pass a client with no custom `Transport`, or set `Proxy: http.ProxyFromEnvironment`.

---

## Output

`FetchSearchQuery` returns a `*SearchResults`. Results are grouped **per page**
(`[][]*SearchResult`), so the outer slice is the page and the inner slice is the
hits on that page.

```json
{
  "query": "best programming language in 2026",
  "total_results": 20,
  "results": [
    [
      {
        "title": "TIOBE Index",
        "url": "https://www.tiobe.com/tiobe-index/",
        "display_url": "https://www.tiobe.com › tiobe-index",
        "description": "Interestingly, three of today's top five languages ...",
        "site_title": "TIOBE",
        "anon_view_url": "https://eu3-browse.startpage.com/av/proxy?ep=...",
        "position": 1
      }
    ]
  ]
}
```

### Fields

| Field           | JSON key        | Description                                                           |
| --------------- | --------------- | -------------------------------------------------------------------- |
| `Title`         | `title`         | Result heading                                                       |
| `URL`           | `url`           | Destination link                                                    |
| `DisplayURL`    | `display_url`   | Breadcrumb-style URL shown in the result (uses `›` separators)      |
| `Description`   | `description`   | Result snippet (HTML-cleaned)                                       |
| `SiteTitle`     | `site_title`    | Source site name                                                   |
| `AnonViewURL`   | `anon_view_url` | Startpage "Anonymous View" proxy link; omitted when not offered     |
| `Position`      | `position`      | 1-based rank **within its page** (resets to 1 on each new page)      |

> **Heads up:** `position` is per-page, not global. For a global rank, combine the
> page index with the position (e.g. `pageIndex*resultsPerPage + position`).

---

## Notes

- This is an unofficial scraper that depends on Startpage's HTML structure and
  request flow, both of which can change without notice and may break parsing.
- Startpage rate-limits and may serve challenge/captcha pages to automated
  clients. Spreading requests across proxies and reusing a per-session cookie jar
  helps avoid this.

---

## License

Released under the [MIT License](LICENSE).
