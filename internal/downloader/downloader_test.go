package downloader

import (
"context"
"fmt"
"net/http"
"net/http/httptest"
"os"
"path/filepath"
"testing"
"time"

"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
"go.uber.org/zap"
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

func TestParseSizeStringFloat64(t *testing.T) {
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
result := parseSizeStringFloat64(tt.size)
// Use tolerance for floating point comparison
tolerance := 0.01
if result < tt.expected-tolerance || result > tt.expected+tolerance {
t.Errorf("parseSizeStringFloat64(%s) = %v, want %v", tt.size, result, tt.expected)
}
})
}
}

func TestParseSizeStringInt64(t *testing.T) {
tests := []struct {
input    string
expected int64
}{
{"5 MB", 5 * 1024 * 1024},
{"5.2 MB", 5452595},
{"1024 KB", 1024 * 1024},
{"1 GB", 1024 * 1024 * 1024},
{"invalid", 0},
{"", 0},
}

for _, tt := range tests {
result := parseSizeStringInt64(tt.input)
if result != tt.expected {
t.Errorf("parseSizeStringInt64(%q) = %d, want %d", tt.input, result, tt.expected)
}
}
}

func TestSanitizeFilename(t *testing.T) {
tests := []struct {
input    string
expected string
}{
{"Normal Book Title", "Normal Book Title"},
{"Book/Title:With*Invalid?Chars", "BookTitleWithInvalidChars"},
{"Book   With   Spaces", "Book   With   Spaces"},
{"Book_With-Dots.txt", "Book_With-Dots.txt"},
}

for _, tt := range tests {
result := sanitizeFilename(tt.input)
if result != tt.expected {
t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
}
}
}

func TestDownloadURL(t *testing.T) {
// Create a test HTTP server
content := []byte("test book content")
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
w.WriteHeader(http.StatusOK)
w.Write(content)
}))
defer server.Close()

// Create temporary directory for test
tmpDir, err := os.MkdirTemp("", "downloader-test-*")
if err != nil {
t.Fatal(err)
}
defer os.RemoveAll(tmpDir)

// Create test config
cfg := &config.Config{
TmpDir:    tmpDir,
IngestDir: tmpDir,
}

logger, _ := zap.NewDevelopment()
downloader := NewDownloader(cfg, logger)

// Test download
outputPath := filepath.Join(tmpDir, "test.txt")
ctx := context.Background()

progressCalled := false
progressCallback := func(progress float64) {
progressCalled = true
if progress < 0 || progress > 100 {
t.Errorf("Invalid progress value: %f", progress)
}
}

err = downloader.DownloadURL(ctx, server.URL, outputPath, "", progressCallback)
if err != nil {
t.Fatalf("DownloadURL failed: %v", err)
}

// Verify file exists and has correct content
downloadedContent, err := os.ReadFile(outputPath)
if err != nil {
t.Fatalf("Failed to read downloaded file: %v", err)
}

if string(downloadedContent) != string(content) {
t.Errorf("Downloaded content = %q, want %q", downloadedContent, content)
}

if !progressCalled {
t.Error("Progress callback was not called")
}
}

func TestDownloadURLCancellation(t *testing.T) {
// Create a test HTTP server that sends data slowly
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
for i := 0; i < 100; i++ {
w.Write([]byte("x"))
time.Sleep(50 * time.Millisecond)
}
}))
defer server.Close()

// Create temporary directory for test
tmpDir, err := os.MkdirTemp("", "downloader-test-*")
if err != nil {
t.Fatal(err)
}
defer os.RemoveAll(tmpDir)

// Create test config
cfg := &config.Config{
TmpDir:    tmpDir,
IngestDir: tmpDir,
}

logger, _ := zap.NewDevelopment()
downloader := NewDownloader(cfg, logger)

// Create context that will be cancelled
ctx, cancel := context.WithCancel(context.Background())

// Cancel after short delay
go func() {
time.Sleep(100 * time.Millisecond)
cancel()
}()

outputPath := filepath.Join(tmpDir, "test.txt")
err = downloader.DownloadURL(ctx, server.URL, outputPath, "", nil)

if err == nil {
t.Error("Expected cancellation error, got nil")
}

// Verify temp file was cleaned up
if _, err := os.Stat(outputPath + TempDownloadExt); err == nil {
t.Error("Temp file was not cleaned up")
}
}

func TestDownloadBook(t *testing.T) {
// Create a test HTTP server
content := []byte("test book content")
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
w.WriteHeader(http.StatusOK)
w.Write(content)
}))
defer server.Close()

// Create temporary directory for test
tmpDir, err := os.MkdirTemp("", "downloader-test-*")
if err != nil {
t.Fatal(err)
}
defer os.RemoveAll(tmpDir)

ingestDir := filepath.Join(tmpDir, "ingest")
if err := os.MkdirAll(ingestDir, 0755); err != nil {
t.Fatal(err)
}

// Create test config
format := "epub"
size := "17 B"
cfg := &config.Config{
TmpDir:       tmpDir,
IngestDir:    ingestDir,
UseBookTitle: true,
}

logger, _ := zap.NewDevelopment()
downloader := NewDownloader(cfg, logger)

// Create book info
book := &models.BookInfo{
ID:           "test123",
Title:        "Test Book",
Format:       &format,
Size:         &size,
DownloadURLs: []string{server.URL},
}

ctx := context.Background()
downloadedPath, err := downloader.DownloadBook(ctx, book, nil)
if err != nil {
t.Fatalf("DownloadBook failed: %v", err)
}

// Verify file is in ingest directory
if !filepath.IsAbs(downloadedPath) {
t.Error("Downloaded path is not absolute")
}

if filepath.Dir(downloadedPath) != ingestDir {
t.Errorf("Downloaded path = %s, want to be in %s", downloadedPath, ingestDir)
}

// Verify content
downloadedContent, err := os.ReadFile(downloadedPath)
if err != nil {
t.Fatalf("Failed to read downloaded file: %v", err)
}

if string(downloadedContent) != string(content) {
t.Errorf("Downloaded content = %q, want %q", downloadedContent, content)
}
}

func TestDownloadBookWithMultipleURLs(t *testing.T) {
content := []byte("test book content")

// Create a server that fails
failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
}))
defer failServer.Close()

// Create a server that succeeds
successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
w.WriteHeader(http.StatusOK)
w.Write(content)
}))
defer successServer.Close()

// Create temporary directory for test
tmpDir, err := os.MkdirTemp("", "downloader-test-*")
if err != nil {
t.Fatal(err)
}
defer os.RemoveAll(tmpDir)

ingestDir := filepath.Join(tmpDir, "ingest")
if err := os.MkdirAll(ingestDir, 0755); err != nil {
t.Fatal(err)
}

// Create test config
format := "epub"
cfg := &config.Config{
TmpDir:    tmpDir,
IngestDir: ingestDir,
}

logger, _ := zap.NewDevelopment()
downloader := NewDownloader(cfg, logger)

// Create book info with failing URL first, then succeeding URL
book := &models.BookInfo{
ID:     "test123",
Title:  "Test Book",
Format: &format,
DownloadURLs: []string{
failServer.URL,
successServer.URL,
},
}

ctx := context.Background()
downloadedPath, err := downloader.DownloadBook(ctx, book, nil)
if err != nil {
t.Fatalf("DownloadBook failed: %v", err)
}

// Verify file exists
if _, err := os.Stat(downloadedPath); err != nil {
t.Errorf("Downloaded file does not exist: %v", err)
}
}

func TestHTMLGetPage_InvalidURL(t *testing.T) {
// This test would require mocking or a test server
t.Skip("Requires test HTTP server")
}

func TestDownloadURLToBuffer_InvalidURL(t *testing.T) {
// This test would require mocking or a test server
t.Skip("Requires test HTTP server")
}
