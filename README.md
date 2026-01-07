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
- Automatically handles session tokens and challenge pages
- Extracts:
    - Title
    - URL
    - Display URL
    - Description
    - Site title
    - Anonymous view URL (when available)
- Clean, structured output
- JSON export

---

## Installation

```bash
go get github.com/yx-zero/startpage-scraper
