package downloader

import (
	"testing"
)

func TestGetAbsoluteURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		relURL   string
		expected string
		wantErr  bool
	}{
		{
			name:     "Absolute URL",
			baseURL:  "https://example.com",
			relURL:   "https://other.com/page",
			expected: "https://other.com/page",
			wantErr:  false,
		},
		{
			name:     "Relative path",
			baseURL:  "https://example.com/base",
			relURL:   "/path/to/page",
			expected: "https://example.com/path/to/page",
			wantErr:  false,
		},
		{
			name:     "Empty URL",
			baseURL:  "https://example.com",
			relURL:   "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Hash only",
			baseURL:  "https://example.com",
			relURL:   "#",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Relative to current directory",
			baseURL:  "https://example.com/dir/",
			relURL:   "page.html",
			expected: "https://example.com/dir/page.html",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetAbsoluteURL(tt.baseURL, tt.relURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAbsoluteURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("GetAbsoluteURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseSizeString(t *testing.T) {
	tests := []struct {
		name     string
		size     string
		expected float64
	}{
		{
			name:     "Megabytes",
			size:     "5.2 MB",
			expected: 5.2 * 1024 * 1024,
		},
		{
			name:     "Kilobytes",
			size:     "500 KB",
			expected: 500 * 1024,
		},
		{
			name:     "Gigabytes",
			size:     "1.5 GB",
			expected: 1.5 * 1024 * 1024 * 1024,
		},
		{
			name:     "With comma as decimal separator",
			size:     "3,5 MB",
			expected: 3.5 * 1024 * 1024,
		},
		{
			name:     "Invalid format",
			size:     "invalid",
			expected: 0,
		},
		{
			name:     "Bytes",
			size:     "1024 B",
			expected: 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSizeString(tt.size)
			// Use tolerance for floating point comparison
			tolerance := 0.01
			if result < tt.expected-tolerance || result > tt.expected+tolerance {
				t.Errorf("parseSizeString(%s) = %v, want %v", tt.size, result, tt.expected)
			}
		})
	}
}

func TestHTMLGetPage_InvalidURL(t *testing.T) {
	// This test would require mocking or a test server
	t.Skip("Requires test HTTP server")
}

func TestDownloadURL_InvalidURL(t *testing.T) {
	// This test would require mocking or a test server
	t.Skip("Requires test HTTP server")
}
