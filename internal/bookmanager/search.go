package bookmanager

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/downloader"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"golang.org/x/net/html"
)

const (
	// textNodeType is the node type for text nodes in the HTML DOM
	textNodeType = html.TextNode
)

// SearchBooks searches for books matching the query
func SearchBooks(ctx context.Context, cfg *config.Config, query string, filters models.SearchFilters) ([]models.BookInfo, error) {
	queryHTML := url.QueryEscape(query)

	// Handle ISBN filters
	if len(filters.ISBN) > 0 {
		var isbnParts []string
		for _, isbn := range filters.ISBN {
			isbnParts = append(isbnParts, fmt.Sprintf("('isbn13:%s' || 'isbn10:%s')", isbn, isbn))
		}
		isbns := strings.Join(isbnParts, " || ")
		queryHTML = url.QueryEscape(fmt.Sprintf("(%s) %s", isbns, query))
	}

	filtersQuery := ""

	// Handle language filters
	bookLanguages := filters.Lang
	if len(bookLanguages) == 0 {
		bookLanguages = strings.Split(strings.ToLower(cfg.BookLanguage), ",")
	}
	for _, value := range bookLanguages {
		if value != "all" {
			filtersQuery += "&lang=" + url.QueryEscape(value)
		}
	}

	// Handle sort filter
	if filters.Sort != nil {
		filtersQuery += "&sort=" + url.QueryEscape(*filters.Sort)
	}

	// Handle content filter
	for _, value := range filters.Content {
		filtersQuery += "&content=" + url.QueryEscape(value)
	}

	// Handle format filter
	formatsToUse := filters.Format
	if len(formatsToUse) == 0 {
		formatsToUse = strings.Split(strings.ToLower(cfg.SupportedFormats), ",")
	}

	// Handle author and title filters
	index := 1
	for _, author := range filters.Author {
		filtersQuery += fmt.Sprintf("&termtype_%d=author&termval_%d=%s", index, index, url.QueryEscape(author))
		index++
	}
	for _, title := range filters.Title {
		filtersQuery += fmt.Sprintf("&termtype_%d=title&termval_%d=%s", index, index, url.QueryEscape(title))
		index++
	}

	// Build URL
	searchURL := fmt.Sprintf(
		"%s/search?index=&page=1&display=table&acc=aa_download&acc=external_download&ext=%s&q=%s%s",
		cfg.AABaseURL,
		strings.Join(formatsToUse, "&ext="),
		queryHTML,
		filtersQuery,
	)

	// Fetch HTML page
	html, err := downloader.HTMLGetPage(ctx, cfg, searchURL, false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch search results: %w", err)
	}

	if strings.Contains(html, "No files found.") {
		return nil, fmt.Errorf("no books found. Please try another query")
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Find results table
	table := doc.Find("table").First()
	if table.Length() == 0 {
		return nil, fmt.Errorf("no books found. Please try another query")
	}

	// Parse results
	var books []models.BookInfo
	table.Find("tr").Each(func(i int, row *goquery.Selection) {
		book, err := parseSearchResultRow(row)
		if err == nil && book != nil {
			books = append(books, *book)
		}
	})

	// Sort by format preference
	sortedFormats := strings.Split(strings.ToLower(cfg.SupportedFormats), ",")
	sort.Slice(books, func(i, j int) bool {
		formatI := ""
		formatJ := ""
		if books[i].Format != nil {
			formatI = *books[i].Format
		}
		if books[j].Format != nil {
			formatJ = *books[j].Format
		}

		indexI := indexOf(sortedFormats, formatI)
		indexJ := indexOf(sortedFormats, formatJ)

		if indexI == -1 {
			indexI = len(sortedFormats)
		}
		if indexJ == -1 {
			indexJ = len(sortedFormats)
		}

		return indexI < indexJ
	})

	return books, nil
}

// parseSearchResultRow parses a single search result row into a BookInfo object
func parseSearchResultRow(row *goquery.Selection) (*models.BookInfo, error) {
	cells := row.Find("td")
	if cells.Length() < 11 {
		return nil, fmt.Errorf("invalid row structure")
	}

	// Get preview image
	var preview *string
	if img := cells.Eq(0).Find("img"); img.Length() > 0 {
		if src, exists := img.Attr("src"); exists {
			preview = &src
		}
	}

	// Get book ID from first link
	links := row.Find("a")
	if links.Length() == 0 {
		return nil, fmt.Errorf("no links found in row")
	}
	href, exists := links.First().Attr("href")
	if !exists {
		return nil, fmt.Errorf("no href found")
	}
	parts := strings.Split(href, "/")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid href")
	}
	id := parts[len(parts)-1]

	// Helper function to extract text from cell
	getText := func(cellIndex int) *string {
		span := cells.Eq(cellIndex).Find("span")
		if span.Length() > 0 {
			// Get the next sibling text node or text content
			node := span.Get(0).NextSibling
			if node != nil && node.Type == textNodeType {
				text := strings.TrimSpace(node.Data)
				if text != "" {
					return &text
				}
			}
			// If no text node sibling, try getting the cell text without the span
			cellText := cells.Eq(cellIndex).Text()
			spanText := span.Text()
			text := strings.TrimSpace(strings.Replace(cellText, spanText, "", 1))
			if text != "" {
				return &text
			}
		}
		return nil
	}

	title := getText(1)
	author := getText(2)
	publisher := getText(3)
	year := getText(4)
	language := getText(7)
	format := getText(9)
	size := getText(10)

	// Title is required, return error if not found
	if title == nil {
		return nil, fmt.Errorf("title not found")
	}

	// Convert format to lowercase
	if format != nil {
		lower := strings.ToLower(*format)
		format = &lower
	}

	return &models.BookInfo{
		ID:        id,
		Preview:   preview,
		Title:     *title,
		Author:    author,
		Publisher: publisher,
		Year:      year,
		Language:  language,
		Format:    format,
		Size:      size,
	}, nil
}

// indexOf returns the index of a string in a slice, or -1 if not found
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
