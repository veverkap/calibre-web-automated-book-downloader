package bookmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/downloader"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
)

// GetBookInfo retrieves detailed information for a specific book
func GetBookInfo(ctx context.Context, cfg *config.Config, bookID string) (*models.BookInfo, error) {
	url := fmt.Sprintf("%s/md5/%s", cfg.AABaseURL, bookID)
	html, err := downloader.HTMLGetPage(ctx, cfg, url, false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch book info for ID %s: %w", bookID, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return parseBookInfoPage(ctx, cfg, doc, bookID)
}

// parseBookInfoPage parses the book info page HTML into a BookInfo object
func parseBookInfoPage(ctx context.Context, cfg *config.Config, doc *goquery.Document, bookID string) (*models.BookInfo, error) {
	// Get preview image
	var preview *string
	if img := doc.Find("body > main > div:nth-of-type(1) div:nth-of-type(1) > img"); img.Length() > 0 {
		if src, exists := img.Attr("src"); exists {
			preview = &src
		}
	}

	// Find the main content div
	mainInner := doc.Find("div.main-inner").First()
	if mainInner.Length() == 0 {
		return nil, fmt.Errorf("failed to parse book info for ID: %s", bookID)
	}

	contentDiv := mainInner.Next()

	// Extract all links to find download URLs
	slowURLsNoWaitlist := make(map[string]bool)
	slowURLsWithWaitlist := make(map[string]bool)
	externalURLsLibgen := make(map[string]bool)
	externalURLsZLib := make(map[string]bool)

	doc.Find("a").Each(func(i int, link *goquery.Selection) {
		text := strings.TrimSpace(strings.ToLower(link.Text()))
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		// Check for slow partner server links
		if strings.HasPrefix(text, "slow partner server") {
			// Check next siblings for waitlist info
			nextText := ""
			if next := link.Next(); next.Length() > 0 {
				nextText = strings.TrimSpace(strings.ToLower(next.Text()))
			}
			if strings.Contains(nextText, "waitlist") {
				if strings.Contains(nextText, "no waitlist") {
					slowURLsNoWaitlist[href] = true
				} else {
					slowURLsWithWaitlist[href] = true
				}
			}
		} else if strings.Contains(text, "click \"get\" at the top") {
			// LibGen links - replace domain
			libgenURL := regexp.MustCompile(`libgen\.(lc|is|bz|st)`).ReplaceAllString(href, "libgen.gl")
			externalURLsLibgen[libgenURL] = true
		} else if strings.HasPrefix(text, "z-lib") {
			if !strings.Contains(href, ".onion/") {
				externalURLsZLib[href] = true
			}
		}
	})

	// Get WELIB URLs if configured
	externalURLsWELIB := make(map[string]bool)
	if cfg.UseCFBypass && cfg.AllowUseWELIB {
		welibURLs, err := getDownloadURLsFromWELIB(ctx, cfg, bookID)
		if err == nil {
			for _, u := range welibURLs {
				externalURLsWELIB[u] = true
			}
		}
	}

	// Build download URLs in priority order
	var urls []string
	if cfg.PrioritizeWELIB {
		urls = appendMapKeys(urls, externalURLsWELIB)
	}
	if cfg.UseCFBypass {
		urls = appendMapKeys(urls, slowURLsNoWaitlist)
	}
	urls = appendMapKeys(urls, externalURLsLibgen)
	if !cfg.PrioritizeWELIB {
		urls = appendMapKeys(urls, externalURLsWELIB)
	}
	if cfg.UseCFBypass {
		urls = appendMapKeys(urls, slowURLsWithWaitlist)
	}
	urls = appendMapKeys(urls, externalURLsZLib)

	// Convert to absolute URLs
	for i := range urls {
		absURL, err := downloader.GetAbsoluteURL(cfg.AABaseURL, urls[i])
		if err == nil && absURL != "" {
			urls[i] = absURL
		}
	}

	// Remove empty URLs
	var filteredURLs []string
	for _, u := range urls {
		if u != "" {
			filteredURLs = append(filteredURLs, u)
		}
	}

	// Parse text content from divs
	var divTexts []string
	var originalDivs []*goquery.Selection
	contentDiv.Children().Each(func(i int, div *goquery.Selection) {
		originalDivs = append(originalDivs, div)
		text := strings.TrimSpace(div.Text())
		if text != "" {
			divTexts = append(divTexts, text)
		}
	})

	// Find separator index (contains ·)
	separatorIndex := 6
	for i, text := range divTexts {
		if strings.Contains(text, "·") {
			separatorIndex = i
			break
		}
	}

	// Parse format and size from separator line
	var format, size string
	if separatorIndex < len(divTexts) {
		details := strings.Split(strings.ToLower(divTexts[separatorIndex]), " · ")
		supportedFormats := strings.Split(strings.ToLower(cfg.SupportedFormats), ",")

		for _, detail := range details {
			detail = strings.TrimSpace(detail)
			// Check for format
			if format == "" {
				for _, sf := range supportedFormats {
					if detail == sf {
						format = detail
						break
					}
				}
			}
			// Check for size
			if size == "" {
				lowerDetail := strings.ToLower(detail)
				if strings.Contains(lowerDetail, "mb") || strings.Contains(lowerDetail, "kb") || strings.Contains(lowerDetail, "gb") {
					size = detail
				}
			}
		}

		// Fallback for format and size
		if format == "" || size == "" {
			for _, detail := range details {
				detail = strings.TrimSpace(detail)
				if format == "" && !strings.Contains(detail, " ") {
					format = detail
				}
				if size == "" && strings.Contains(detail, ".") {
					size = detail
				}
			}
		}
	}

	// Extract title, author, publisher
	var title, author, publisher string
	if separatorIndex >= 3 && separatorIndex < len(divTexts) {
		title = strings.Trim(divTexts[separatorIndex-3], "🔍")
		author = divTexts[separatorIndex-2]
		publisher = divTexts[separatorIndex-1]
	}

	// Extract metadata
	var info map[string][]string
	if len(originalDivs) >= 6 {
		info = extractBookMetadata(originalDivs[len(originalDivs)-6])
	}

	// Create BookInfo
	bookInfo := &models.BookInfo{
		ID:           bookID,
		Preview:      preview,
		Title:        title,
		DownloadURLs: filteredURLs,
		Info:         info,
	}

	if author != "" {
		bookInfo.Author = &author
	}
	if publisher != "" {
		bookInfo.Publisher = &publisher
	}
	if format != "" {
		bookInfo.Format = &format
	}
	if size != "" {
		bookInfo.Size = &size
	}

	// Set language and year from metadata if available
	if info != nil {
		if lang, ok := info["Language"]; ok && len(lang) > 0 {
			bookInfo.Language = &lang[0]
		}
		if year, ok := info["Year"]; ok && len(year) > 0 {
			bookInfo.Year = &year[0]
		}
	}

	return bookInfo, nil
}

// getDownloadURLsFromWELIB retrieves download URLs from welib.org
func getDownloadURLsFromWELIB(ctx context.Context, cfg *config.Config, bookID string) ([]string, error) {
	if !cfg.AllowUseWELIB {
		return nil, nil
	}

	url := fmt.Sprintf("https://welib.org/md5/%s", bookID)
	html, err := downloader.HTMLGetPage(ctx, cfg, url, true)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var downloadLinks []string
	doc.Find("a[href]").Each(func(i int, link *goquery.Selection) {
		href, exists := link.Attr("href")
		if !exists {
			return
		}
		if strings.Contains(href, "/slow_download/") {
			absURL, err := downloader.GetAbsoluteURL(url, href)
			if err == nil && absURL != "" {
				downloadLinks = append(downloadLinks, absURL)
			}
		}
	})

	return downloadLinks, nil
}

// extractBookMetadata extracts metadata from book info divs
func extractBookMetadata(metadataDiv *goquery.Selection) map[string][]string {
	info := make(map[string][]string)

	// Find nested divs with metadata
	metadataDiv.Find("div").First().Children().Each(func(i int, div *goquery.Selection) {
		text := strings.TrimSpace(div.Text())
		if text == "" {
			return
		}

		// Each metadata item has two children: key and value
		children := div.Children()
		if children.Length() < 2 {
			return
		}

		key := strings.TrimSpace(children.Eq(0).Text())
		value := strings.TrimSpace(children.Eq(1).Text())

		if key != "" && value != "" {
			if _, exists := info[key]; !exists {
				info[key] = []string{}
			}
			info[key] = append(info[key], value)
		}
	})

	// Filter relevant metadata
	relevantPrefixes := []string{
		"ISBN-",
		"ALTERNATIVE",
		"ASIN",
		"Goodreads",
		"Language",
		"Year",
	}

	filtered := make(map[string][]string)
	for key, values := range info {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "filename") {
			continue
		}

		for _, prefix := range relevantPrefixes {
			if strings.HasPrefix(lowerKey, strings.ToLower(prefix)) {
				filtered[strings.TrimSpace(key)] = values
				break
			}
		}
	}

	return filtered
}

// appendMapKeys appends map keys to a slice
func appendMapKeys(slice []string, m map[string]bool) []string {
	for key := range m {
		slice = append(slice, key)
	}
	return slice
}

// DownloadBook downloads a book from available sources
func DownloadBook(ctx context.Context, cfg *config.Config, bookInfo *models.BookInfo, progressCallback func(float64)) ([]byte, error) {
	// If download URLs are not set, fetch book info first
	if len(bookInfo.DownloadURLs) == 0 {
		fullInfo, err := GetBookInfo(ctx, cfg, bookInfo.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get book info: %w", err)
		}
		bookInfo.DownloadURLs = fullInfo.DownloadURLs
	}

	downloadLinks := make([]string, len(bookInfo.DownloadURLs))
	copy(downloadLinks, bookInfo.DownloadURLs)

	// If AA_DONATOR_KEY is set, use the fast download URL
	if cfg.AADonatorKey != "" {
		fastURL := fmt.Sprintf("%s/dyn/api/fast_download.json?md5=%s&key=%s",
			cfg.AABaseURL, bookInfo.ID, cfg.AADonatorKey)
		downloadLinks = append([]string{fastURL}, downloadLinks...)
	}

	// Try each download link
	for _, link := range downloadLinks {
		downloadURL, err := getDownloadURL(ctx, cfg, link, bookInfo.Title)
		if err != nil || downloadURL == "" {
			continue
		}

		size := ""
		if bookInfo.Size != nil {
			size = *bookInfo.Size
		}

		buffer, err := downloader.DownloadURL(ctx, cfg, downloadURL, size, progressCallback)
		if err != nil {
			continue
		}

		return buffer.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to download book from any source")
}

// getDownloadURL extracts actual download URL from various source pages
func getDownloadURL(ctx context.Context, cfg *config.Config, link, title string) (string, error) {
	// Fast download API
	if strings.HasPrefix(link, cfg.AABaseURL+"/dyn/api/fast_download.json") {
		html, err := downloader.HTMLGetPage(ctx, cfg, link, false)
		if err != nil {
			return "", err
		}

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(html), &result); err != nil {
			return "", fmt.Errorf("failed to parse JSON: %w", err)
		}

		if url, ok := result["download_url"].(string); ok {
			return url, nil
		}
		return "", fmt.Errorf("no download_url in response")
	}

	// Regular download pages
	html, err := downloader.HTMLGetPage(ctx, cfg, link, false)
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	var downloadURL string

	// Z-Library
	if strings.HasPrefix(link, "https://z-lib.") {
		if downloadLink := doc.Find("a.addDownloadedBook[href]"); downloadLink.Length() > 0 {
			downloadURL, _ = downloadLink.Attr("href")
		}
	} else if strings.Contains(link, "/slow_download/") {
		// Slow download with countdown
		if downloadLink := doc.Find("a:contains('📚 Download now')"); downloadLink.Length() > 0 {
			downloadURL, _ = downloadLink.Attr("href")
		} else {
			// Check for countdown
			if countdown := doc.Find("span.js-partner-countdown"); countdown.Length() > 0 {
				// Note: Countdown wait logic not implemented in Phase 3
				// This will be implemented in Phase 4 when browser automation is integrated
				// The Python version waits for the countdown and retries the same URL
				return "", fmt.Errorf("download requires countdown wait - will be implemented in Phase 4 with browser automation")
			}
		}
	} else {
		// LibGen and others - find "GET" link
		if getLink := doc.Find("a:contains('GET')"); getLink.Length() > 0 {
			downloadURL, _ = getLink.Attr("href")
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no download link found")
	}

	return downloader.GetAbsoluteURL(link, downloadURL)
}
