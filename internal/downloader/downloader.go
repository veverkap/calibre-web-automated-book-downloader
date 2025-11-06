package downloader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
)

const (
	// expectedSizeRatio is the minimum ratio of downloaded bytes to expected size
	// required to consider the download successful (90%)
	expectedSizeRatio = 0.9
)

// HTTPClient interface for testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultClient is the default HTTP client
var DefaultClient HTTPClient = &http.Client{
	Timeout: 30 * time.Second,
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

// DownloadURL downloads content from URL into a buffer
func DownloadURL(ctx context.Context, cfg *config.Config, link string, size string, progressCallback func(float64)) (*bytes.Buffer, error) {
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
		totalSize = parseSizeString(size)
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

// parseSizeString parses size string like "5.2 MB" to bytes
func parseSizeString(size string) float64 {
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
