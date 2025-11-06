package models

import (
	"testing"
	"time"
)

func TestNewBookQueue(t *testing.T) {
	timeout := 1 * time.Hour
	queue := NewBookQueue(timeout)
	
	if queue == nil {
		t.Fatal("Expected queue to be created")
	}
	
	if queue.statusTimeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, queue.statusTimeout)
	}
}

func TestBookQueueAdd(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	title := "Test Book"
	book := &BookInfo{
		ID:    "test-1",
		Title: title,
	}
	
	queue.Add("test-1", book, 0)
	
	// Check that the book was added
	status := queue.GetStatus()
	if len(status[StatusQueued]) != 1 {
		t.Errorf("Expected 1 queued book, got %d", len(status[StatusQueued]))
	}
	
	if queuedBook, exists := status[StatusQueued]["test-1"]; !exists {
		t.Error("Expected book to be in queue")
	} else if queuedBook.Title != title {
		t.Errorf("Expected title '%s', got '%s'", title, queuedBook.Title)
	}
}

func TestBookQueueGetNext(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{
		ID:    "test-1",
		Title: "Test Book",
	}
	
	queue.Add("test-1", book, 0)
	
	bookID, cancelChan, ok := queue.GetNext()
	if !ok {
		t.Fatal("Expected to get a book from queue")
	}
	
	if bookID != "test-1" {
		t.Errorf("Expected book ID 'test-1', got '%s'", bookID)
	}
	
	if cancelChan == nil {
		t.Error("Expected cancel channel to be created")
	}
}

func TestBookQueuePriority(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	// Add books with different priorities
	book1 := &BookInfo{ID: "test-1", Title: "Book 1"}
	book2 := &BookInfo{ID: "test-2", Title: "Book 2"}
	book3 := &BookInfo{ID: "test-3", Title: "Book 3"}
	
	queue.Add("test-1", book1, 10) // Low priority
	queue.Add("test-2", book2, 1)  // High priority
	queue.Add("test-3", book3, 5)  // Medium priority
	
	// Get books in priority order
	bookID1, _, ok := queue.GetNext()
	if !ok || bookID1 != "test-2" {
		t.Errorf("Expected first book to be 'test-2', got '%s'", bookID1)
	}
	
	bookID2, _, ok := queue.GetNext()
	if !ok || bookID2 != "test-3" {
		t.Errorf("Expected second book to be 'test-3', got '%s'", bookID2)
	}
	
	bookID3, _, ok := queue.GetNext()
	if !ok || bookID3 != "test-1" {
		t.Errorf("Expected third book to be 'test-1', got '%s'", bookID3)
	}
}

func TestBookQueueUpdateStatus(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 0)
	
	queue.UpdateStatus("test-1", StatusDownloading)
	
	status := queue.GetStatus()
	if len(status[StatusDownloading]) != 1 {
		t.Errorf("Expected 1 downloading book, got %d", len(status[StatusDownloading]))
	}
	
	queue.UpdateStatus("test-1", StatusDone)
	
	status = queue.GetStatus()
	if len(status[StatusDone]) != 1 {
		t.Errorf("Expected 1 done book, got %d", len(status[StatusDone]))
	}
}

func TestBookQueueCancelDownload(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 0)
	
	// Cancel a queued book
	success := queue.CancelDownload("test-1")
	if !success {
		t.Error("Expected cancellation to succeed")
	}
	
	status := queue.GetStatus()
	if len(status[StatusCancelled]) != 1 {
		t.Errorf("Expected 1 cancelled book, got %d", len(status[StatusCancelled]))
	}
}

func TestBookQueueSetPriority(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 10)
	
	success := queue.SetPriority("test-1", 1)
	if !success {
		t.Error("Expected priority change to succeed")
	}
	
	// Verify the priority was changed
	order := queue.GetQueueOrder()
	if len(order) != 1 {
		t.Fatalf("Expected 1 item in queue, got %d", len(order))
	}
	
	if order[0].Priority != 1 {
		t.Errorf("Expected priority 1, got %d", order[0].Priority)
	}
}

func TestBookQueueReorderQueue(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book1 := &BookInfo{ID: "test-1", Title: "Book 1"}
	book2 := &BookInfo{ID: "test-2", Title: "Book 2"}
	
	queue.Add("test-1", book1, 10)
	queue.Add("test-2", book2, 20)
	
	// Reorder the queue
	priorities := map[string]int{
		"test-1": 5,
		"test-2": 1,
	}
	
	success := queue.ReorderQueue(priorities)
	if !success {
		t.Error("Expected reorder to succeed")
	}
	
	// Verify the order
	order := queue.GetQueueOrder()
	if len(order) != 2 {
		t.Fatalf("Expected 2 items in queue, got %d", len(order))
	}
	
	// Book 2 should now have higher priority (lower number)
	if order[0].ID != "test-2" {
		t.Errorf("Expected first book to be 'test-2', got '%s'", order[0].ID)
	}
}

func TestBookQueueGetActiveDownloads(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 0)
	
	// Get the book (which marks it as active)
	_, _, _ = queue.GetNext()
	
	activeDownloads := queue.GetActiveDownloads()
	if len(activeDownloads) != 1 {
		t.Errorf("Expected 1 active download, got %d", len(activeDownloads))
	}
	
	if activeDownloads[0] != "test-1" {
		t.Errorf("Expected active download 'test-1', got '%s'", activeDownloads[0])
	}
}

func TestBookQueueClearCompleted(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book1 := &BookInfo{ID: "test-1", Title: "Book 1"}
	book2 := &BookInfo{ID: "test-2", Title: "Book 2"}
	book3 := &BookInfo{ID: "test-3", Title: "Book 3"}
	
	queue.Add("test-1", book1, 0)
	queue.Add("test-2", book2, 0)
	queue.Add("test-3", book3, 0)
	
	queue.UpdateStatus("test-1", StatusDone)
	queue.UpdateStatus("test-2", StatusError)
	queue.UpdateStatus("test-3", StatusQueued)
	
	count := queue.ClearCompleted()
	if count != 2 {
		t.Errorf("Expected 2 books cleared, got %d", count)
	}
	
	status := queue.GetStatus()
	if len(status[StatusQueued]) != 1 {
		t.Errorf("Expected 1 queued book remaining, got %d", len(status[StatusQueued]))
	}
}

func TestBookQueueUpdateProgress(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 0)
	
	queue.UpdateProgress("test-1", 0.5)
	
	status := queue.GetStatus()
	if queuedBook, exists := status[StatusQueued]["test-1"]; !exists {
		t.Error("Expected book to be in queue")
	} else if queuedBook.Progress == nil {
		t.Error("Expected progress to be set")
	} else if *queuedBook.Progress != 0.5 {
		t.Errorf("Expected progress 0.5, got %f", *queuedBook.Progress)
	}
}

func TestBookQueueUpdateDownloadPath(t *testing.T) {
	queue := NewBookQueue(1 * time.Hour)
	
	book := &BookInfo{ID: "test-1", Title: "Test Book"}
	queue.Add("test-1", book, 0)
	
	path := "/path/to/book.epub"
	queue.UpdateDownloadPath("test-1", path)
	
	status := queue.GetStatus()
	if queuedBook, exists := status[StatusQueued]["test-1"]; !exists {
		t.Error("Expected book to be in queue")
	} else if queuedBook.DownloadPath == nil {
		t.Error("Expected download path to be set")
	} else if *queuedBook.DownloadPath != path {
		t.Errorf("Expected path '%s', got '%s'", path, *queuedBook.DownloadPath)
	}
}
