package downloader

import (
	"context"
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

func TestParseSizeString(t *testing.T) {
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
		result := parseSizeString(tt.input)
		if result != tt.expected {
			t.Errorf("parseSizeString(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestDownloadURL(t *testing.T) {
	// Create a test HTTP server
	content := []byte("test book content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "17")
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

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Verify content
	downloadedContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != string(content) {
		t.Errorf("Downloaded content mismatch: got %q, want %q", downloadedContent, content)
	}

	if progressCalled {
		t.Log("Progress callback was called")
	}
}

func TestDownloadURLCancellation(t *testing.T) {
	// Create a test HTTP server that sends data slowly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		
		// Write slowly to allow cancellation
		for i := 0; i < 100; i++ {
			w.Write(make([]byte, 1000))
			time.Sleep(10 * time.Millisecond)
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

	// Test download with cancellation
	outputPath := filepath.Join(tmpDir, "test.txt")
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err = downloader.DownloadURL(ctx, server.URL, outputPath, "", nil)
	if err == nil {
		t.Error("Expected download to be cancelled, but it succeeded")
	}

	// The error could be either "download cancelled" or "context canceled"
	errMsg := err.Error()
	if errMsg != "download cancelled" && errMsg != "failed to read response: context canceled" {
		t.Errorf("Expected cancellation error, got: %v", err)
	}
}

func TestDownloadBook(t *testing.T) {
	// Create a test HTTP server
	content := []byte("test epub content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "17")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	// Create temporary directories for test
	tmpDir, err := os.MkdirTemp("", "downloader-test-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ingestDir, err := os.MkdirTemp("", "downloader-test-ingest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(ingestDir)

	// Create test config
	cfg := &config.Config{
		TmpDir:        tmpDir,
		IngestDir:     ingestDir,
		UseBookTitle:  true,
		CustomScript:  "", // No custom script for test
	}

	logger, _ := zap.NewDevelopment()
	downloader := NewDownloader(cfg, logger)

	// Create test book
	format := "epub"
	book := &models.BookInfo{
		ID:           "test-book-123",
		Title:        "Test Book Title",
		Format:       &format,
		DownloadURLs: []string{server.URL},
	}

	// Test download
	ctx := context.Background()
	progressCalled := false
	progressCallback := func(progress float64) {
		progressCalled = true
	}

	downloadPath, err := downloader.DownloadBook(ctx, book, progressCallback)
	if err != nil {
		t.Fatalf("DownloadBook failed: %v", err)
	}

	// Verify file path
	expectedFilename := "Test Book Title.epub"
	expectedPath := filepath.Join(ingestDir, expectedFilename)
	
	if downloadPath != expectedPath {
		t.Errorf("Download path mismatch: got %q, want %q", downloadPath, expectedPath)
	}

	// Verify file exists
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}

	// Verify content
	downloadedContent, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != string(content) {
		t.Errorf("Downloaded content mismatch: got %q, want %q", downloadedContent, content)
	}

	if progressCalled {
		t.Log("Progress callback was called")
	}
}

func TestDownloadBookWithMultipleURLs(t *testing.T) {
	// Create a test HTTP server that fails
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	// Create a test HTTP server that succeeds
	content := []byte("test content")
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer successServer.Close()

	// Create temporary directories for test
	tmpDir, err := os.MkdirTemp("", "downloader-test-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ingestDir, err := os.MkdirTemp("", "downloader-test-ingest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(ingestDir)

	// Create test config
	cfg := &config.Config{
		TmpDir:    tmpDir,
		IngestDir: ingestDir,
	}

	logger, _ := zap.NewDevelopment()
	downloader := NewDownloader(cfg, logger)

	// Create test book with multiple URLs (first fails, second succeeds)
	format := "txt"
	book := &models.BookInfo{
		ID:     "test-book",
		Title:  "Test",
		Format: &format,
		DownloadURLs: []string{
			failServer.URL,     // This will fail
			successServer.URL,  // This will succeed
		},
	}

	// Test download
	ctx := context.Background()
	downloadPath, err := downloader.DownloadBook(ctx, book, nil)
	if err != nil {
		t.Fatalf("DownloadBook failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}

	// Verify content from successful server
	downloadedContent, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != string(content) {
		t.Errorf("Downloaded content mismatch: got %q, want %q", downloadedContent, content)
	}
}
