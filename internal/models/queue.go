package models

import (
	"container/heap"
	"sync"
	"time"
)

// QueueStatus represents the status of a book in the queue
type QueueStatus string

const (
	StatusQueued      QueueStatus = "queued"
	StatusDownloading QueueStatus = "downloading"
	StatusAvailable   QueueStatus = "available"
	StatusError       QueueStatus = "error"
	StatusDone        QueueStatus = "done"
	StatusCancelled   QueueStatus = "cancelled"
)

// BookInfo represents information about a book
type BookInfo struct {
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	Preview      *string             `json:"preview,omitempty"`
	Author       *string             `json:"author,omitempty"`
	Publisher    *string             `json:"publisher,omitempty"`
	Year         *string             `json:"year,omitempty"`
	Language     *string             `json:"language,omitempty"`
	Format       *string             `json:"format,omitempty"`
	Size         *string             `json:"size,omitempty"`
	Info         map[string][]string `json:"info,omitempty"`
	DownloadURLs []string            `json:"download_urls,omitempty"`
	DownloadPath *string             `json:"download_path,omitempty"`
	Priority     int                 `json:"priority"`
	Progress     *float64            `json:"progress,omitempty"`
}

// SearchFilters represents search filter criteria
type SearchFilters struct {
	ISBN    []string `json:"isbn,omitempty"`
	Author  []string `json:"author,omitempty"`
	Title   []string `json:"title,omitempty"`
	Lang    []string `json:"lang,omitempty"`
	Sort    *string  `json:"sort,omitempty"`
	Content []string `json:"content,omitempty"`
	Format  []string `json:"format,omitempty"`
}

// QueueItem represents an item in the priority queue
type QueueItem struct {
	BookID    string
	Priority  int
	AddedTime time.Time
	Index     int // index in the heap
}

// PriorityQueue implements a priority queue for QueueItems
type PriorityQueue []*QueueItem

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Lower priority number = higher precedence
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	// If priorities are equal, earlier added time comes first
	return pq[i].AddedTime.Before(pq[j].AddedTime)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueueItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.Index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// BookQueue manages a thread-safe priority queue of books
type BookQueue struct {
	mu                sync.RWMutex
	queue             *PriorityQueue
	status            map[string]QueueStatus
	bookData          map[string]*BookInfo
	statusTimestamps  map[string]time.Time
	statusTimeout     time.Duration
	cancelFlags       map[string]chan struct{}
	activeDownloads   map[string]bool
}

// NewBookQueue creates a new BookQueue instance
func NewBookQueue(statusTimeout time.Duration) *BookQueue {
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)
	
	return &BookQueue{
		queue:            &pq,
		status:           make(map[string]QueueStatus),
		bookData:         make(map[string]*BookInfo),
		statusTimestamps: make(map[string]time.Time),
		statusTimeout:    statusTimeout,
		cancelFlags:      make(map[string]chan struct{}),
		activeDownloads:  make(map[string]bool),
	}
}

// Add adds a book to the queue with the specified priority
func (bq *BookQueue) Add(bookID string, bookData *BookInfo, priority int) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	// Don't add if already exists and not in error/done state
	if status, exists := bq.status[bookID]; exists {
		if status != StatusError && status != StatusDone && status != StatusCancelled {
			return
		}
	}

	bookData.Priority = priority
	item := &QueueItem{
		BookID:    bookID,
		Priority:  priority,
		AddedTime: time.Now(),
	}
	
	heap.Push(bq.queue, item)
	bq.bookData[bookID] = bookData
	bq.updateStatus(bookID, StatusQueued)
}

// GetNext retrieves the next book from the queue
func (bq *BookQueue) GetNext() (string, chan struct{}, bool) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	// Loop until we find a non-cancelled item or the queue is empty
	for bq.queue.Len() > 0 {
		item := heap.Pop(bq.queue).(*QueueItem)
		bookID := item.BookID

		// Check if book was cancelled while in queue
		if status, exists := bq.status[bookID]; exists && status == StatusCancelled {
			// Skip cancelled items and continue to next
			continue
		}

		// Create cancellation channel for this download
		cancelChan := make(chan struct{})
		bq.cancelFlags[bookID] = cancelChan
		bq.activeDownloads[bookID] = true

		return bookID, cancelChan, true
	}

	// Queue is empty or all items were cancelled
	return "", nil, false
}

// updateStatus is an internal method to update status and timestamp
func (bq *BookQueue) updateStatus(bookID string, status QueueStatus) {
	bq.status[bookID] = status
	bq.statusTimestamps[bookID] = time.Now()
}

// UpdateStatus updates the status of a book in the queue
func (bq *BookQueue) UpdateStatus(bookID string, status QueueStatus) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	bq.updateStatus(bookID, status)

	// Clean up active download tracking when finished
	if status == StatusAvailable || status == StatusError || status == StatusDone || status == StatusCancelled {
		delete(bq.activeDownloads, bookID)
		if ch, exists := bq.cancelFlags[bookID]; exists {
			close(ch)
			delete(bq.cancelFlags, bookID)
		}
	}
}

// UpdateDownloadPath updates the download path of a book
func (bq *BookQueue) UpdateDownloadPath(bookID string, downloadPath string) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if book, exists := bq.bookData[bookID]; exists {
		book.DownloadPath = &downloadPath
	}
}

// UpdateProgress updates the download progress of a book
func (bq *BookQueue) UpdateProgress(bookID string, progress float64) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if book, exists := bq.bookData[bookID]; exists {
		book.Progress = &progress
	}
}

// GetStatus returns the current queue status
func (bq *BookQueue) GetStatus() map[QueueStatus]map[string]*BookInfo {
	bq.Refresh()
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	result := make(map[QueueStatus]map[string]*BookInfo)
	statuses := []QueueStatus{StatusQueued, StatusDownloading, StatusAvailable, StatusError, StatusDone, StatusCancelled}
	for _, status := range statuses {
		result[status] = make(map[string]*BookInfo)
	}

	for bookID, status := range bq.status {
		if book, exists := bq.bookData[bookID]; exists {
			result[status][bookID] = book
		}
	}

	return result
}

// QueueOrderItem represents an item in the queue order
type QueueOrderItem struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Author    *string     `json:"author,omitempty"`
	Priority  int         `json:"priority"`
	AddedTime time.Time   `json:"added_time"`
	Status    QueueStatus `json:"status"`
}

// GetQueueOrder returns the current queue order
func (bq *BookQueue) GetQueueOrder() []QueueOrderItem {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	var items []QueueOrderItem
	
	// Make a copy of the queue to inspect without modifying
	queueCopy := make([]*QueueItem, bq.queue.Len())
	copy(queueCopy, *bq.queue)

	for _, item := range queueCopy {
		if book, exists := bq.bookData[item.BookID]; exists {
			status, _ := bq.status[item.BookID]
			items = append(items, QueueOrderItem{
				ID:        item.BookID,
				Title:     book.Title,
				Author:    book.Author,
				Priority:  item.Priority,
				AddedTime: item.AddedTime,
				Status:    status,
			})
		}
	}

	return items
}

// CancelDownload cancels a download and marks it as cancelled
func (bq *BookQueue) CancelDownload(bookID string) bool {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	currentStatus, exists := bq.status[bookID]
	if !exists {
		return false
	}

	if currentStatus == StatusDownloading {
		// Signal active download to stop
		if cancelChan, exists := bq.cancelFlags[bookID]; exists {
			close(cancelChan)
			delete(bq.cancelFlags, bookID)
		}
		bq.updateStatus(bookID, StatusCancelled)
		return true
	} else if currentStatus == StatusQueued {
		// Mark as cancelled
		bq.updateStatus(bookID, StatusCancelled)
		return true
	}

	return false
}

// SetPriority changes the priority of a queued book
func (bq *BookQueue) SetPriority(bookID string, newPriority int) bool {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	status, exists := bq.status[bookID]
	if !exists || status != StatusQueued {
		return false
	}

	// Find and update the item in the queue
	for i, item := range *bq.queue {
		if item.BookID == bookID {
			(*bq.queue)[i].Priority = newPriority
			heap.Fix(bq.queue, i)
			
			// Update book data priority
			if book, exists := bq.bookData[bookID]; exists {
				book.Priority = newPriority
			}
			return true
		}
	}

	return false
}

// ReorderQueue bulk reorders the queue by setting new priorities
func (bq *BookQueue) ReorderQueue(bookPriorities map[string]int) bool {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	// Update priorities in the queue
	for i, item := range *bq.queue {
		if newPriority, exists := bookPriorities[item.BookID]; exists {
			(*bq.queue)[i].Priority = newPriority
			
			// Update book data priority
			if book, exists := bq.bookData[item.BookID]; exists {
				book.Priority = newPriority
			}
		}
	}

	// Re-heapify the queue
	heap.Init(bq.queue)

	return true
}

// GetActiveDownloads returns a list of currently active download book IDs
func (bq *BookQueue) GetActiveDownloads() []string {
	bq.mu.RLock()
	defer bq.mu.RUnlock()

	downloads := make([]string, 0, len(bq.activeDownloads))
	for bookID := range bq.activeDownloads {
		downloads = append(downloads, bookID)
	}

	return downloads
}

// ClearCompleted removes all completed, errored, or cancelled books
func (bq *BookQueue) ClearCompleted() int {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	var toRemove []string
	for bookID, status := range bq.status {
		if status == StatusDone || status == StatusError || status == StatusCancelled {
			toRemove = append(toRemove, bookID)
		}
	}

	for _, bookID := range toRemove {
		delete(bq.status, bookID)
		delete(bq.statusTimestamps, bookID)
		delete(bq.bookData, bookID)
		if ch, exists := bq.cancelFlags[bookID]; exists {
			close(ch)
			delete(bq.cancelFlags, bookID)
		}
		delete(bq.activeDownloads, bookID)
	}

	return len(toRemove)
}

// Refresh removes books with stale status
func (bq *BookQueue) Refresh() {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	currentTime := time.Now()
	var toRemove []string

	for bookID, status := range bq.status {
		// Check if download path exists
		if book, exists := bq.bookData[bookID]; exists && book.DownloadPath != nil {
			// In a real implementation, you'd check if the file exists
			// For now, we'll skip this check
		}

		// Check for completed downloads
		if status == StatusAvailable {
			if book, exists := bq.bookData[bookID]; exists && book.DownloadPath == nil {
				bq.updateStatus(bookID, StatusDone)
			}
		}

		// Check for stale status entries
		if lastUpdate, exists := bq.statusTimestamps[bookID]; exists {
			if currentTime.Sub(lastUpdate) > bq.statusTimeout {
				if status == StatusDone || status == StatusError || status == StatusAvailable || status == StatusCancelled {
					toRemove = append(toRemove, bookID)
				}
			}
		}
	}

	// Remove stale entries
	for _, bookID := range toRemove {
		delete(bq.status, bookID)
		delete(bq.statusTimestamps, bookID)
		delete(bq.bookData, bookID)
	}
}

// SetStatusTimeout sets the status timeout duration
func (bq *BookQueue) SetStatusTimeout(timeout time.Duration) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	bq.statusTimeout = timeout
}
