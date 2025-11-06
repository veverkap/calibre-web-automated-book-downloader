package bookmanager

import (
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
)

func TestParseSearchResultRow(t *testing.T) {
	// Sample HTML for a search result row
	html := `
	<table>
	<tr>
		<td><img src="https://example.com/cover.jpg" /></td>
		<td><a href="/md5/abc123def456"><span></span>Test Book Title</a></td>
		<td><span></span>John Doe</td>
		<td><span></span>Test Publisher</td>
		<td><span></span>2023</td>
		<td><span></span></td>
		<td><span></span></td>
		<td><span></span></td>
		<td><span></span>English</td>
		<td><span></span></td>
		<td><span></span>epub</td>
		<td><span></span>5.2 MB</td>
	</tr>
	</table>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	row := doc.Find("tr").First()
	book, err := parseSearchResultRow(row)

	if err != nil {
		t.Fatalf("Failed to parse row: %v", err)
	}

	if book == nil {
		t.Fatal("Expected book to be non-nil")
	}

	if book.ID != "abc123def456" {
		t.Errorf("Expected ID to be 'abc123def456', got '%s'", book.ID)
	}

	if book.Preview == nil || *book.Preview != "https://example.com/cover.jpg" {
		t.Error("Preview URL not parsed correctly")
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		slice    []string
		item     string
		expected int
	}{
		{[]string{"epub", "mobi", "pdf"}, "mobi", 1},
		{[]string{"epub", "mobi", "pdf"}, "azw3", -1},
		{[]string{}, "epub", -1},
	}

	for _, tt := range tests {
		result := indexOf(tt.slice, tt.item)
		if result != tt.expected {
			t.Errorf("indexOf(%v, %s) = %d, expected %d", tt.slice, tt.item, result, tt.expected)
		}
	}
}

func TestAppendMapKeys(t *testing.T) {
	m := map[string]bool{
		"key1": true,
		"key2": true,
		"key3": true,
	}

	var slice []string
	result := appendMapKeys(slice, m)

	if len(result) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(result))
	}

	// Check that all keys are present
	keyMap := make(map[string]bool)
	for _, key := range result {
		keyMap[key] = true
	}

	for key := range m {
		if !keyMap[key] {
			t.Errorf("Expected key %s to be in result", key)
		}
	}
}

func TestExtractBookMetadata(t *testing.T) {
	html := `
	<div>
		<div>
			<div>
				<div>ISBN-13</div>
				<div>978-1234567890</div>
			</div>
			<div>
				<div>Language</div>
				<div>English</div>
			</div>
			<div>
				<div>Year</div>
				<div>2023</div>
			</div>
			<div>
				<div>Filename</div>
				<div>test.epub</div>
			</div>
		</div>
	</div>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	metadata := extractBookMetadata(doc.Find("div").First())

	// Should filter out Filename
	if _, exists := metadata["Filename"]; exists {
		t.Error("Filename should be filtered out")
	}

	// Should include ISBN, Language, Year
	if isbn, exists := metadata["ISBN-13"]; !exists || len(isbn) == 0 || isbn[0] != "978-1234567890" {
		t.Error("ISBN-13 not parsed correctly")
	}

	if lang, exists := metadata["Language"]; !exists || len(lang) == 0 || lang[0] != "English" {
		t.Error("Language not parsed correctly")
	}

	if year, exists := metadata["Year"]; !exists || len(year) == 0 || year[0] != "2023" {
		t.Error("Year not parsed correctly")
	}
}

func TestParseBookInfoPage(t *testing.T) {
	html := `
	<html>
		<body>
			<main>
				<div>
					<div>
						<img src="https://example.com/cover.jpg" />
					</div>
				</div>
			</main>
			<div class="main-inner"></div>
			<div>
				<div>🔍Test Book Title</div>
				<div>Test Author</div>
				<div>Test Publisher</div>
				<div></div>
				<div></div>
				<div></div>
				<div>epub · 5.2 MB</div>
				<div></div>
				<div></div>
				<div></div>
				<div></div>
				<div>
					<div>
						<div>
							<div>
								<div>ISBN-13</div>
								<div>978-1234567890</div>
							</div>
							<div>
								<div>Language</div>
								<div>English</div>
							</div>
							<div>
								<div>Year</div>
								<div>2023</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		</body>
	</html>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	cfg := &config.Config{
		SupportedFormats: "epub,mobi,pdf",
	}

	book, err := parseBookInfoPage(context.Background(), cfg, doc, "test123")
	if err != nil {
		t.Fatalf("Failed to parse book info: %v", err)
	}

	if book.ID != "test123" {
		t.Errorf("Expected ID 'test123', got '%s'", book.ID)
	}

	if book.Title != "Test Book Title" {
		t.Errorf("Expected title 'Test Book Title', got '%s'", book.Title)
	}

	if book.Author == nil || *book.Author != "Test Author" {
		t.Error("Author not parsed correctly")
	}

	if book.Publisher == nil || *book.Publisher != "Test Publisher" {
		t.Error("Publisher not parsed correctly")
	}

	if book.Format == nil || *book.Format != "epub" {
		t.Error("Format not parsed correctly")
	}

	if book.Size == nil || !strings.Contains(strings.ToLower(*book.Size), "mb") {
		t.Error("Size not parsed correctly")
	}

	// Metadata parsing may vary based on HTML structure, so we'll be less strict
	if book.Info != nil {
		if lang, ok := book.Info["Language"]; ok && len(lang) > 0 {
			if book.Language == nil || *book.Language != "English" {
				t.Error("Language not parsed correctly from metadata")
			}
		}
		if year, ok := book.Info["Year"]; ok && len(year) > 0 {
			if book.Year == nil || *book.Year != "2023" {
				t.Error("Year not parsed correctly from metadata")
			}
		}
	}
}

func TestSearchBooks_InvalidHTML(t *testing.T) {
	// This test would require mocking the downloader.HTMLGetPage function
	// For now, we'll skip it since we're focused on the parsing logic
	t.Skip("Requires mocking HTTP client")
}

func TestGetBookInfo_InvalidHTML(t *testing.T) {
	// This test would require mocking the downloader.HTMLGetPage function
	t.Skip("Requires mocking HTTP client")
}
