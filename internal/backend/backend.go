package backend

import (
	"fmt"
	"os"

	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

// Backend provides high-level business logic for the application
type Backend struct {
	queue  *models.BookQueue
	logger *zap.Logger
}

// NewBackend creates a new Backend instance
func NewBackend(queue *models.BookQueue, logger *zap.Logger) *Backend {
	return &Backend{
		queue:  queue,
		logger: logger,
	}
}

// QueueBook adds a book to the download queue
func (b *Backend) QueueBook(bookID string, bookInfo *models.BookInfo, priority int) error {
	if bookInfo == nil {
		return fmt.Errorf("book info is required")
	}

	b.queue.Add(bookID, bookInfo, priority)
	b.logger.Info("Book queued",
		zap.String("book_id", bookID),
		zap.String("title", bookInfo.Title),
		zap.Int("priority", priority))

	return nil
}

// GetQueueStatus returns the current queue status
func (b *Backend) GetQueueStatus() map[models.QueueStatus]map[string]*models.BookInfo {
	status := b.queue.GetStatus()

	// Verify download paths still exist
	for _, books := range status {
		for _, book := range books {
			if book.DownloadPath != nil {
				if _, err := os.Stat(*book.DownloadPath); os.IsNotExist(err) {
					book.DownloadPath = nil
				}
			}
		}
	}

	return status
}

// GetBookData retrieves the downloaded book data
func (b *Backend) GetBookData(bookID string) ([]byte, *models.BookInfo, error) {
	status := b.queue.GetStatus()

	// Find the book in any status
	var book *models.BookInfo
	for _, books := range status {
		if b, exists := books[bookID]; exists {
			book = b
			break
		}
	}

	if book == nil {
		return nil, nil, fmt.Errorf("book not found: %s", bookID)
	}

	if book.DownloadPath == nil || *book.DownloadPath == "" {
		return nil, book, fmt.Errorf("book not downloaded yet: %s", bookID)
	}

	data, err := os.ReadFile(*book.DownloadPath)
	if err != nil {
		// Clear the download path if file doesn't exist
		if os.IsNotExist(err) {
			book.DownloadPath = nil
		}
		return nil, book, fmt.Errorf("failed to read book data: %w", err)
	}

	return data, book, nil
}

// CancelDownload cancels a download
func (b *Backend) CancelDownload(bookID string) bool {
	success := b.queue.CancelDownload(bookID)
	if success {
		b.logger.Info("Download cancelled", zap.String("book_id", bookID))
	}
	return success
}

// SetBookPriority changes the priority of a queued book
func (b *Backend) SetBookPriority(bookID string, priority int) bool {
	success := b.queue.SetPriority(bookID, priority)
	if success {
		b.logger.Info("Priority updated",
			zap.String("book_id", bookID),
			zap.Int("priority", priority))
	}
	return success
}

// ReorderQueue bulk reorders the queue
func (b *Backend) ReorderQueue(bookPriorities map[string]int) bool {
	return b.queue.ReorderQueue(bookPriorities)
}

// GetQueueOrder returns the current queue order
func (b *Backend) GetQueueOrder() []models.QueueOrderItem {
	return b.queue.GetQueueOrder()
}

// GetActiveDownloads returns list of currently active downloads
func (b *Backend) GetActiveDownloads() []string {
	return b.queue.GetActiveDownloads()
}

// ClearCompleted removes all completed downloads from tracking
func (b *Backend) ClearCompleted() int {
	count := b.queue.ClearCompleted()
	b.logger.Info("Cleared completed downloads", zap.Int("count", count))
	return count
}
