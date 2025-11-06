package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

// ProgressCallback is a function that receives download progress updates (0-100)
type ProgressCallback func(progress float64)

// Downloader handles book download operations
type Downloader struct {
	config     *config.Config
	logger     *zap.Logger
	httpClient *http.Client
}

// NewDownloader creates a new Downloader instance
func NewDownloader(cfg *config.Config, logger *zap.Logger) *Downloader {
	// Create HTTP client with proxy support if configured
	transport := &http.Transport{}
	
	if cfg.HTTPProxy != "" || cfg.HTTPSProxy != "" {
		// Proxy configuration would go here
		// For now, using default transport
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // No timeout for downloads, we'll handle cancellation
	}

	return &Downloader{
		config:     cfg,
		logger:     logger,
		httpClient: client,
	}
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(filename string) string {
	// Keep only alphanumeric, spaces, dots, and underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9 ._-]`)
	sanitized := reg.ReplaceAllString(filename, "")
	return strings.TrimSpace(sanitized)
}

// DownloadURL downloads content from a URL with progress tracking and cancellation support
func (d *Downloader) DownloadURL(ctx context.Context, url string, outputPath string, size string, progressCallback ProgressCallback) error {
	d.logger.Info("Downloading from URL", zap.String("url", url), zap.String("output", outputPath))

	// Create HTTP request with context for cancellation
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("download cancelled")
		}
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Determine total size
	var totalSize int64
	if size != "" {
		// Parse size string (e.g., "5.2 MB")
		totalSize = parseSizeString(size)
	}
	if totalSize == 0 {
		totalSize = resp.ContentLength
	}

	// Create output file
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Download with progress tracking
	var downloaded int64
	buf := make([]byte, 32*1024) // 32KB buffer
	lastProgress := time.Now()
	
	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			d.logger.Info("Download cancelled", zap.String("url", url))
			return fmt.Errorf("download cancelled")
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write to file: %w", writeErr)
			}
			downloaded += int64(n)

			// Update progress periodically
			if progressCallback != nil && totalSize > 0 {
				now := time.Now()
				if now.Sub(lastProgress) >= time.Second {
					progress := float64(downloaded) * 100.0 / float64(totalSize)
					progressCallback(progress)
					lastProgress = now
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
	}

	// Final progress update
	if progressCallback != nil && totalSize > 0 {
		progressCallback(100.0)
	}

	// Validate download size
	if totalSize > 0 && downloaded < int64(float64(totalSize)*0.9) {
		// Check if we got HTML instead of binary content
		if resp.Header.Get("Content-Type") != "" && 
		   strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			return fmt.Errorf("received HTML content instead of book file")
		}
	}

	d.logger.Info("Download completed", 
		zap.String("url", url),
		zap.Int64("size", downloaded))

	return nil
}

// parseSizeString parses a size string like "5.2 MB" into bytes
func parseSizeString(size string) int64 {
	// Clean up the string
	size = strings.TrimSpace(size)
	size = strings.ReplaceAll(size, ",", ".")
	size = strings.ReplaceAll(size, " ", "")
	size = strings.ToUpper(size)

	// Extract number and unit
	var value float64
	var unit string

	if strings.HasSuffix(size, "MB") {
		unit = "MB"
		size = strings.TrimSuffix(size, "MB")
	} else if strings.HasSuffix(size, "KB") {
		unit = "KB"
		size = strings.TrimSuffix(size, "KB")
	} else if strings.HasSuffix(size, "GB") {
		unit = "GB"
		size = strings.TrimSuffix(size, "GB")
	}

	value, err := strconv.ParseFloat(size, 64)
	if err != nil {
		return 0
	}

	// Convert to bytes
	switch unit {
	case "KB":
		return int64(value * 1024)
	case "MB":
		return int64(value * 1024 * 1024)
	case "GB":
		return int64(value * 1024 * 1024 * 1024)
	default:
		return int64(value)
	}
}

// DownloadBook downloads a book and processes it according to configuration
func (d *Downloader) DownloadBook(ctx context.Context, book *models.BookInfo, progressCallback ProgressCallback) (string, error) {
	d.logger.Info("Starting book download", 
		zap.String("title", book.Title),
		zap.String("id", book.ID))

	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		d.logger.Info("Download cancelled before starting", zap.String("id", book.ID))
		return "", fmt.Errorf("download cancelled")
	default:
	}

	// Determine filename
	var bookName string
	if d.config.UseBookTitle && book.Title != "" {
		bookName = sanitizeFilename(book.Title)
	} else {
		bookName = book.ID
	}

	// Add format extension
	format := "epub" // default
	if book.Format != nil && *book.Format != "" {
		format = *book.Format
	}
	bookName = fmt.Sprintf("%s.%s", bookName, format)

	// Create temporary file path
	tmpPath := filepath.Join(d.config.TmpDir, bookName)

	// Ensure tmp directory exists
	if err := os.MkdirAll(d.config.TmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create tmp directory: %w", err)
	}

	// Download from first available URL
	var downloadErr error
	for i, url := range book.DownloadURLs {
		d.logger.Info("Attempting download", 
			zap.Int("attempt", i+1),
			zap.Int("total", len(book.DownloadURLs)),
			zap.String("url", url))

		size := ""
		if book.Size != nil {
			size = *book.Size
		}

		downloadErr = d.DownloadURL(ctx, url, tmpPath, size, progressCallback)
		if downloadErr == nil {
			break
		}

		d.logger.Warn("Download attempt failed",
			zap.Error(downloadErr),
			zap.Int("attempt", i+1))
	}

	if downloadErr != nil {
		return "", fmt.Errorf("all download attempts failed: %w", downloadErr)
	}

	// Check for cancellation before post-processing
	select {
	case <-ctx.Done():
		d.logger.Info("Download cancelled before post-processing", zap.String("id", book.ID))
		os.Remove(tmpPath)
		return "", fmt.Errorf("download cancelled")
	default:
	}

	// Run custom script if configured
	if d.config.CustomScript != "" {
		d.logger.Info("Running custom script", 
			zap.String("script", d.config.CustomScript),
			zap.String("file", tmpPath))

		cmd := exec.Command(d.config.CustomScript, tmpPath)
		if err := cmd.Run(); err != nil {
			d.logger.Warn("Custom script failed", zap.Error(err))
			// Don't fail the download if custom script fails
		}
	}

	// Ensure ingest directory exists
	if err := os.MkdirAll(d.config.IngestDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create ingest directory: %w", err)
	}

	// Move to ingest directory via intermediate path
	intermediatePath := filepath.Join(d.config.IngestDir, fmt.Sprintf("%s.crdownload", book.ID))
	finalPath := filepath.Join(d.config.IngestDir, bookName)

	// Move to intermediate path first
	if err := os.Rename(tmpPath, intermediatePath); err != nil {
		// If rename fails, try copy and delete
		d.logger.Debug("Rename failed, trying copy", zap.Error(err))
		if err := copyFile(tmpPath, intermediatePath); err != nil {
			return "", fmt.Errorf("failed to move file to ingest directory: %w", err)
		}
		os.Remove(tmpPath)
	}

	// Final cancellation check before completing
	select {
	case <-ctx.Done():
		d.logger.Info("Download cancelled before final rename", zap.String("id", book.ID))
		os.Remove(intermediatePath)
		return "", fmt.Errorf("download cancelled")
	default:
	}

	// Rename to final name
	if err := os.Rename(intermediatePath, finalPath); err != nil {
		return "", fmt.Errorf("failed to rename to final path: %w", err)
	}

	d.logger.Info("Download completed successfully",
		zap.String("title", book.Title),
		zap.String("path", finalPath))

	return finalPath, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
