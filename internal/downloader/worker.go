package downloader

import (
	"context"
	"sync"
	"time"

	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

// WorkerPool manages concurrent book downloads using goroutines
type WorkerPool struct {
	config     *config.Config
	logger     *zap.Logger
	downloader *Downloader
	queue      *models.BookQueue
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// NewWorkerPool creates a new download worker pool
func NewWorkerPool(cfg *config.Config, logger *zap.Logger, queue *models.BookQueue) *WorkerPool {
	return &WorkerPool{
		config:     cfg,
		logger:     logger,
		downloader: NewDownloader(cfg, logger),
		queue:      queue,
		stopChan:   make(chan struct{}),
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() {
	wp.logger.Info("Starting download worker pool",
		zap.Int("max_workers", wp.config.MaxConcurrentDownloads))

	// Start worker goroutines
	for i := 0; i < wp.config.MaxConcurrentDownloads; i++ {
		wp.wg.Add(1)
		go wp.worker(i + 1)
	}
}

// Stop gracefully stops the worker pool
func (wp *WorkerPool) Stop() {
	wp.logger.Info("Stopping download worker pool")
	close(wp.stopChan)
	wp.wg.Wait()
	wp.logger.Info("Download worker pool stopped")
}

// worker is a goroutine that processes downloads
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	wp.logger.Info("Worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-wp.stopChan:
			wp.logger.Info("Worker stopping", zap.Int("worker_id", id))
			return
		default:
			// Try to get next book from queue
			bookID, cancelChan, ok := wp.queue.GetNext()
			if !ok {
				// Queue is empty, sleep briefly and retry
				time.Sleep(time.Duration(wp.config.MainLoopSleepTime) * time.Second)
				continue
			}

			wp.logger.Info("Worker processing download",
				zap.Int("worker_id", id),
				zap.String("book_id", bookID))

			// Process the download
			wp.processDownload(bookID, cancelChan)
		}
	}
}

// processDownload processes a single book download
func (wp *WorkerPool) processDownload(bookID string, cancelChan chan struct{}) {
	// Create context from cancel channel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Monitor cancel channel in a goroutine
	go func() {
		select {
		case <-cancelChan:
			wp.logger.Info("Download cancellation requested", zap.String("book_id", bookID))
			cancel()
		case <-ctx.Done():
		}
	}()

	// Update status to downloading
	wp.queue.UpdateStatus(bookID, models.StatusDownloading)

	// Get book info from queue
	status := wp.queue.GetStatus()
	var book *models.BookInfo
	for _, statusBooks := range status {
		if b, exists := statusBooks[bookID]; exists {
			book = b
			break
		}
	}

	if book == nil {
		wp.logger.Error("Book not found in queue", zap.String("book_id", bookID))
		wp.queue.UpdateStatus(bookID, models.StatusError)
		return
	}

	// Create progress callback
	progressCallback := func(progress float64) {
		wp.queue.UpdateProgress(bookID, progress)
	}

	// Attempt download
	downloadPath, err := wp.downloader.DownloadBook(ctx, book, progressCallback)

	// Check if cancelled
	select {
	case <-ctx.Done():
		wp.logger.Info("Download cancelled", zap.String("book_id", bookID))
		wp.queue.UpdateStatus(bookID, models.StatusCancelled)
		return
	default:
	}

	// Update queue based on result
	if err != nil {
		wp.logger.Error("Download failed",
			zap.String("book_id", bookID),
			zap.Error(err))
		wp.queue.UpdateStatus(bookID, models.StatusError)
		return
	}

	// Success
	wp.queue.UpdateDownloadPath(bookID, downloadPath)
	wp.queue.UpdateStatus(bookID, models.StatusAvailable)

	wp.logger.Info("Download completed successfully",
		zap.String("book_id", bookID),
		zap.String("path", downloadPath))
}
