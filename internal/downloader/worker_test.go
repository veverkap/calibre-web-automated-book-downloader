package downloader

import (
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

func TestWorkerPoolIntegration(t *testing.T) {
	// Create test HTTP servers for downloads
	content1 := []byte("test book content 1")
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "19")
		w.WriteHeader(http.StatusOK)
		w.Write(content1)
	}))
	defer server1.Close()

	content2 := []byte("test book content 2")
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "19")
		w.WriteHeader(http.StatusOK)
		w.Write(content2)
	}))
	defer server2.Close()

	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "worker-test-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ingestDir, err := os.MkdirTemp("", "worker-test-ingest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(ingestDir)

	// Create test config
	cfg := &config.Config{
		TmpDir:                 tmpDir,
		IngestDir:              ingestDir,
		UseBookTitle:           false,
		MaxConcurrentDownloads: 2,
		MainLoopSleepTime:      1, // 1 second sleep
		StatusTimeout:          3600,
	}

	logger, _ := zap.NewDevelopment()
	queue := models.NewBookQueue(time.Duration(cfg.StatusTimeout) * time.Second)

	// Create and start worker pool
	workerPool := NewWorkerPool(cfg, logger, queue)
	workerPool.Start()
	defer workerPool.Stop()

	// Add books to queue
	format := "txt"
	book1 := &models.BookInfo{
		ID:           "book-1",
		Title:        "Test Book 1",
		Format:       &format,
		DownloadURLs: []string{server1.URL},
	}
	queue.Add("book-1", book1, 0)

	book2 := &models.BookInfo{
		ID:           "book-2",
		Title:        "Test Book 2",
		Format:       &format,
		DownloadURLs: []string{server2.URL},
	}
	queue.Add("book-2", book2, 0)

	// Wait for downloads to complete (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	book1Complete := false
	book2Complete := false

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for downloads to complete")
		case <-ticker.C:
			status := queue.GetStatus()
			
			// Check if both books are in available or error status
			if _, exists := status[models.StatusAvailable]["book-1"]; exists {
				book1Complete = true
			}
			if _, exists := status[models.StatusError]["book-1"]; exists {
				t.Fatal("Book 1 download failed")
			}
			
			if _, exists := status[models.StatusAvailable]["book-2"]; exists {
				book2Complete = true
			}
			if _, exists := status[models.StatusError]["book-2"]; exists {
				t.Fatal("Book 2 download failed")
			}
			
			if book1Complete && book2Complete {
				// Success! Both downloads completed
				goto completed
			}
		}
	}

completed:
	t.Log("Both downloads completed successfully")

	// Verify files exist
	status := queue.GetStatus()
	
	book1Result, exists := status[models.StatusAvailable]["book-1"]
	if !exists {
		t.Fatal("Book 1 not in available status")
	}
	if book1Result.DownloadPath == nil {
		t.Fatal("Book 1 download path is nil")
	}
	
	if _, err := os.Stat(*book1Result.DownloadPath); os.IsNotExist(err) {
		t.Errorf("Book 1 file does not exist: %s", *book1Result.DownloadPath)
	}

	book2Result, exists := status[models.StatusAvailable]["book-2"]
	if !exists {
		t.Fatal("Book 2 not in available status")
	}
	if book2Result.DownloadPath == nil {
		t.Fatal("Book 2 download path is nil")
	}
	
	if _, err := os.Stat(*book2Result.DownloadPath); os.IsNotExist(err) {
		t.Errorf("Book 2 file does not exist: %s", *book2Result.DownloadPath)
	}

	// Verify file contents
	data1, err := os.ReadFile(*book1Result.DownloadPath)
	if err != nil {
		t.Fatalf("Failed to read book 1: %v", err)
	}
	if string(data1) != string(content1) {
		t.Errorf("Book 1 content mismatch: got %q, want %q", data1, content1)
	}

	data2, err := os.ReadFile(*book2Result.DownloadPath)
	if err != nil {
		t.Fatalf("Failed to read book 2: %v", err)
	}
	if string(data2) != string(content2) {
		t.Errorf("Book 2 content mismatch: got %q, want %q", data2, content2)
	}
}

func TestWorkerPoolCancellation(t *testing.T) {
	// Create a slow test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		
		// Write slowly to allow cancellation
		for i := 0; i < 100; i++ {
			w.Write(make([]byte, 1000))
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()

	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "worker-test-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ingestDir, err := os.MkdirTemp("", "worker-test-ingest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(ingestDir)

	// Create test config
	cfg := &config.Config{
		TmpDir:                 tmpDir,
		IngestDir:              ingestDir,
		MaxConcurrentDownloads: 1,
		MainLoopSleepTime:      1,
		StatusTimeout:          3600,
	}

	logger, _ := zap.NewDevelopment()
	queue := models.NewBookQueue(time.Duration(cfg.StatusTimeout) * time.Second)

	// Create and start worker pool
	workerPool := NewWorkerPool(cfg, logger, queue)
	workerPool.Start()
	defer workerPool.Stop()

	// Add book to queue
	format := "txt"
	book := &models.BookInfo{
		ID:           "slow-book",
		Title:        "Slow Book",
		Format:       &format,
		DownloadURLs: []string{server.URL},
	}
	queue.Add("slow-book", book, 0)

	// Wait a bit for download to start
	time.Sleep(200 * time.Millisecond)

	// Cancel the download
	success := queue.CancelDownload("slow-book")
	if !success {
		t.Error("Failed to cancel download")
	}

	// Wait for cancellation to take effect
	time.Sleep(500 * time.Millisecond)

	// Verify book is cancelled
	status := queue.GetStatus()
	if _, exists := status[models.StatusCancelled]["slow-book"]; !exists {
		// Could also be downloading if cancellation hasn't processed yet
		if _, exists := status[models.StatusDownloading]["slow-book"]; !exists {
			t.Error("Book not in cancelled or downloading status")
		}
	}

	// Verify file was not created or was cleaned up
	expectedPath := filepath.Join(ingestDir, "slow-book.txt")
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Error("Download file should not exist after cancellation")
	}
}
