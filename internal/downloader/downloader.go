package downloader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

const (
	// MinDownloadSizeRatio is the minimum acceptable ratio of downloaded size to expected size
	MinDownloadSizeRatio = 0.9
	// TempDownloadExt is the extension used for files being downloaded
	TempDownloadExt = ".crdownload"
	// expectedSizeRatio is the minimum ratio of downloaded bytes to expected size
	// required to consider the download successful (90%) - used for buffer downloads
	expectedSizeRatio = 0.9
)

// ProgressCallback is a function that receives download progress updates (0-100)
type ProgressCallback func(progress float64)

// HTTPClient interface for testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultClient is the default HTTP client
var DefaultClient HTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

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

// HTMLGetPage fetches HTML content from a URL with retry mechanism
func HTMLGetPage(ctx context.Context, cfg *config.Config, urlStr string, useBypasser bool) (string, error) {
	return htmlGetPageRetry(ctx, cfg, urlStr, cfg.MaxRetry, useBypasser)
}

// htmlGetPageRetry internal function with retry logic
func htmlGetPageRetry(ctx context.Context, cfg *config.Config, urlStr string, retry int, useBypasser bool) (string, error) {
	// TODO: Implement Cloudflare bypasser integration when useBypasser is true
	if useBypasser && cfg.UseCFBypass {
		// For now, we'll fall through to regular HTTP request
		// This will be implemented in Phase 4
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.3")

	// Create client with proxy if configured
	client := createHTTPClient(cfg)

	resp, err := client.Do(req)
	if err != nil {
		if retry == 0 {
			return "", fmt.Errorf("failed to fetch page: %w", err)
		}
		sleepTime := time.Duration(cfg.DefaultSleep*(cfg.MaxRetry-retry+1)) * time.Second
		time.Sleep(sleepTime)
		return htmlGetPageRetry(ctx, cfg, urlStr, retry-1, useBypasser)
	}
	defer resp.Body.Close()

	// Handle specific status codes
	if resp.StatusCode == 404 {
		return "", fmt.Errorf("404 error for URL: %s", urlStr)
	}

	if resp.StatusCode == 403 {
		// 403 detected, should retry using cloudflare bypass
		if retry > 0 {
			return htmlGetPageRetry(ctx, cfg, urlStr, retry-1, true)
		}
		return "", fmt.Errorf("403 error for URL: %s", urlStr)
	}

	if resp.StatusCode != 200 {
		if retry == 0 {
			return "", fmt.Errorf("unexpected status code %d for URL: %s", resp.StatusCode, urlStr)
		}
		sleepTime := time.Duration(cfg.DefaultSleep*(cfg.MaxRetry-retry+1)) * time.Second
		time.Sleep(sleepTime)
		return htmlGetPageRetry(ctx, cfg, urlStr, retry-1, useBypasser)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Small delay to be respectful
	time.Sleep(1 * time.Second)

	return string(body), nil
}

// DownloadURL downloads content from a URL with progress tracking and cancellation support (method on Downloader)
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
		totalSize = parseSizeStringInt64(size)
	}
	if totalSize == 0 {
		totalSize = resp.ContentLength
	}

	// Create temporary file for download
	tempPath := outputPath + TempDownloadExt
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download with progress tracking
	var downloaded int64
	buffer := make([]byte, 32*1024) // 32KB buffer

	for {
		select {
		case <-ctx.Done():
			// Cleanup temp file on cancellation
			os.Remove(tempPath)
			return fmt.Errorf("download cancelled")
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := file.Write(buffer[:n])
			if writeErr != nil {
				os.Remove(tempPath)
				return fmt.Errorf("failed to write to file: %w", writeErr)
			}
			downloaded += int64(n)

			// Report progress
			if progressCallback != nil && totalSize > 0 {
				progressCallback(float64(downloaded) * 100.0 / float64(totalSize))
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("failed to read from response: %w", err)
		}
	}

	// Close file before renaming
	file.Close()

	// Validate download size
	if totalSize > 0 && float64(downloaded) < float64(totalSize)*MinDownloadSizeRatio {
		os.Remove(tempPath)
		return fmt.Errorf("incomplete download: got %d bytes, expected %d", downloaded, totalSize)
	}

	// Rename temp file to final path
	if err := os.Rename(tempPath, outputPath); err != nil {
		// Try copy if rename fails (cross-device link)
		if copyErr := copyFile(tempPath, outputPath); copyErr != nil {
			os.Remove(tempPath)
			return fmt.Errorf("failed to move file: %w", err)
		}
		os.Remove(tempPath)
	}

	d.logger.Info("Download complete", zap.String("path", outputPath), zap.Int64("size", downloaded))
	return nil
}

// DownloadURLToBuffer downloads content from URL into a buffer (standalone function for bookmanager)
func DownloadURLToBuffer(ctx context.Context, cfg *config.Config, link string, size string, progressCallback func(float64)) (*bytes.Buffer, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Create client with proxy if configured
	client := createHTTPClient(cfg)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from %s: %w", link, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d for URL: %s", resp.StatusCode, link)
	}

	// Parse expected size
	totalSize := float64(resp.ContentLength)
	if totalSize == 0 && size != "" {
		// Try to parse size from string (e.g., "5.2 MB")
		totalSize = parseSizeStringFloat64(size)
	}

	buffer := new(bytes.Buffer)
	downloaded := float64(0)

	// Read in chunks
	chunk := make([]byte, 1000)
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("download cancelled: %s", link)
		default:
		}

		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buffer.Write(chunk[:n])
			downloaded += float64(n)
			if progressCallback != nil && totalSize > 0 {
				progressCallback(downloaded * 100.0 / totalSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read from %s: %w", link, err)
		}
	}

	// Validate that we downloaded enough data
	// If we received less than 90% of the expected size, check if it's an error page
	if totalSize > 0 && downloaded < totalSize*expectedSizeRatio {
		contentType := resp.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "text/html") {
			return nil, fmt.Errorf("failed to download content for %s. Found HTML content instead", link)
		}
	}

	return buffer, nil
}

// parseSizeStringInt64 parses size string like "5.2 MB" to bytes as int64
func parseSizeStringInt64(size string) int64 {
	size = strings.TrimSpace(size)
	size = strings.ToUpper(size)
	size = strings.ReplaceAll(size, ",", ".")

	var value float64
	var unit string

	// Try to parse format like "5.2 MB"
	_, err := fmt.Sscanf(size, "%f %s", &value, &unit)
	if err != nil {
		return 0
	}

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

// parseSizeStringFloat64 parses size string like "5.2 MB" to bytes as float64
func parseSizeStringFloat64(size string) float64 {
	size = strings.TrimSpace(size)
	size = strings.ToUpper(size)
	size = strings.ReplaceAll(size, ",", ".")

	var value float64
	var unit string

	// Try to parse format like "5.2 MB"
	_, err := fmt.Sscanf(size, "%f %s", &value, &unit)
	if err != nil {
		return 0
	}

	switch unit {
	case "KB":
		return value * 1024
	case "MB":
		return value * 1024 * 1024
	case "GB":
		return value * 1024 * 1024 * 1024
	default:
		return value
	}
}

// GetAbsoluteURL converts relative URL to absolute URL
func GetAbsoluteURL(baseURL, relURL string) (string, error) {
	relURL = strings.TrimSpace(relURL)
	if relURL == "" || relURL == "#" {
		return "", nil
	}

	if strings.HasPrefix(relURL, "http") {
		return relURL, nil
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	rel, err := url.Parse(relURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse relative URL: %w", err)
	}

	return base.ResolveReference(rel).String(), nil
}

// createHTTPClient creates an HTTP client with proxy configuration
func createHTTPClient(cfg *config.Config) *http.Client {
	transport := &http.Transport{}

	// Configure proxy
	if cfg.HTTPProxy != "" || cfg.HTTPSProxy != "" {
		transport.Proxy = func(req *http.Request) (*url.URL, error) {
			if req.URL.Scheme == "https" && cfg.HTTPSProxy != "" {
				return url.Parse(cfg.HTTPSProxy)
			}
			if req.URL.Scheme == "http" && cfg.HTTPProxy != "" {
				return url.Parse(cfg.HTTPProxy)
			}
			return nil, nil
		}
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}

// DownloadBook downloads a book using the provided book info (method on Downloader)
func (d *Downloader) DownloadBook(ctx context.Context, book *models.BookInfo, progressCallback ProgressCallback) (string, error) {
	if len(book.DownloadURLs) == 0 {
		return "", fmt.Errorf("no download URLs available for book: %s", book.Title)
	}

	// Add donator key URL if configured
	urls := make([]string, 0, len(book.DownloadURLs)+1)
	if d.config.AADonatorKey != "" {
		fastURL := fmt.Sprintf("%s/dyn/api/fast_download.json?md5=%s&key=%s",
			d.config.AABaseURL, book.ID, d.config.AADonatorKey)
		urls = append(urls, fastURL)
	}
	urls = append(urls, book.DownloadURLs...)

	// Determine output filename
	filename := book.Title
	if d.config.UseBookTitle && book.Title != "" {
		filename = book.Title
	} else {
		filename = book.ID
	}

	// Add format extension if available
	if book.Format != nil && *book.Format != "" {
		filename = fmt.Sprintf("%s.%s", filename, *book.Format)
	}

	// Sanitize filename
	filename = sanitizeFilename(filename)

	// Create output path
	outputPath := filepath.Join(d.config.TmpDir, filename)

	// Try each URL until one succeeds
	var lastErr error
	for _, downloadURL := range urls {
		d.logger.Info("Attempting download", zap.String("url", downloadURL))

		size := ""
		if book.Size != nil {
			size = *book.Size
		}

		err := d.DownloadURL(ctx, downloadURL, outputPath, size, progressCallback)
		if err == nil {
			// Download successful
			// Execute custom script if configured
			if d.config.CustomScript != "" {
				d.logger.Info("Executing custom script", zap.String("script", d.config.CustomScript))
				cmd := exec.CommandContext(ctx, d.config.CustomScript, outputPath)
				if err := cmd.Run(); err != nil {
					d.logger.Error("Custom script failed", zap.Error(err))
					// Don't fail the download if script fails
				}
			}

			// Move to ingest directory
			finalPath := filepath.Join(d.config.IngestDir, filename)
			if err := os.Rename(outputPath, finalPath); err != nil {
				// Try copy if rename fails
				if copyErr := copyFile(outputPath, finalPath); copyErr != nil {
					return "", fmt.Errorf("failed to move file to ingest dir: %w", err)
				}
				os.Remove(outputPath)
			}

			d.logger.Info("Book download complete", zap.String("path", finalPath))
			return finalPath, nil
		}

		lastErr = err
		d.logger.Warn("Download failed, trying next URL", zap.Error(err))
	}

	return "", fmt.Errorf("all download attempts failed, last error: %w", lastErr)
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
