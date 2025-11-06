package api

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

// handleSearch handles book search requests
// GET /api/search
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	
	filters := &models.SearchFilters{}
	
	// Parse filters from query parameters
	if isbn := query["isbn"]; len(isbn) > 0 {
		filters.ISBN = isbn
	}
	if author := query["author"]; len(author) > 0 {
		filters.Author = author
	}
	if title := query["title"]; len(title) > 0 {
		filters.Title = title
	}
	if lang := query["lang"]; len(lang) > 0 {
		filters.Lang = lang
	}
	if sort := query.Get("sort"); sort != "" {
		filters.Sort = &sort
	}
	if content := query["content"]; len(content) > 0 {
		filters.Content = content
	}
	if format := query["format"]; len(format) > 0 {
		filters.Format = format
	}

	h.logger.Info("Search request", zap.Any("filters", filters))

	// TODO: Implement actual search logic
	// For now, return a placeholder response
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"message": "Search functionality not yet implemented",
		"filters": filters,
		"results": []interface{}{},
	})
}

// handleInfo handles book info requests
// GET /api/info?id=<book_id>
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		h.writeError(w, http.StatusBadRequest, "Missing book ID")
		return
	}

	h.logger.Info("Info request", zap.String("book_id", bookID))

	// TODO: Implement actual book info retrieval
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"message": "Book info functionality not yet implemented",
		"book_id": bookID,
	})
}

// handleDownload handles download requests
// GET /api/download?id=<book_id>&priority=<priority>
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		h.writeError(w, http.StatusBadRequest, "Missing book ID")
		return
	}

	priority := 0
	if p := r.URL.Query().Get("priority"); p != "" {
		var err error
		priority, err = strconv.Atoi(p)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "Invalid priority value")
			return
		}
	}

	h.logger.Info("Download request", 
		zap.String("book_id", bookID),
		zap.Int("priority", priority))

	// TODO: Add book to download queue
	// For now, return a placeholder response
	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"message": "Download queued",
		"book_id": bookID,
		"priority": priority,
	})
}

// handleStatus handles status requests
// GET /api/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := h.backend.GetQueueStatus()
	
	h.logger.Info("Status request")

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"queue_status": status,
	})
}

// handleLocalDownload handles local file download
// GET /api/localdownload?id=<book_id>
func (h *Handler) handleLocalDownload(w http.ResponseWriter, r *http.Request) {
	bookID := r.URL.Query().Get("id")
	if bookID == "" {
		h.writeError(w, http.StatusBadRequest, "Missing book ID")
		return
	}

	h.logger.Info("Local download request", zap.String("book_id", bookID))

	// Get book data
	data, book, err := h.backend.GetBookData(bookID)
	if err != nil {
		h.logger.Error("Failed to get book data",
			zap.String("book_id", bookID),
			zap.Error(err))
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Set appropriate headers
	filename := book.Title
	if book.Format != nil && *book.Format != "" {
		filename = filename + "." + *book.Format
	}

	// Escape filename to prevent header injection
	escapedFilename := mime.QEncoding.Encode("utf-8", filename)
	w.Header().Set("Content-Disposition", "attachment; filename*=utf-8''"+escapedFilename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleCancelDownload handles download cancellation
// DELETE /api/download/{book_id}/cancel
func (h *Handler) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "book_id")
	if bookID == "" {
		h.writeError(w, http.StatusBadRequest, "Missing book ID")
		return
	}

	h.logger.Info("Cancel download request", zap.String("book_id", bookID))

	success := h.backend.CancelDownload(bookID)
	
	if success {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "success",
			"message": "Download cancelled",
			"book_id": bookID,
		})
	} else {
		h.writeError(w, http.StatusNotFound, "Book not found or cannot be cancelled")
	}
}

// handleSetPriority handles priority update requests
// PUT /api/queue/{book_id}/priority
func (h *Handler) handleSetPriority(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "book_id")
	if bookID == "" {
		h.writeError(w, http.StatusBadRequest, "Missing book ID")
		return
	}

	var req struct {
		Priority int `json:"priority"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.logger.Info("Set priority request",
		zap.String("book_id", bookID),
		zap.Int("priority", req.Priority))

	success := h.backend.SetBookPriority(bookID, req.Priority)
	
	if success {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "success",
			"message": "Priority updated",
			"book_id": bookID,
			"priority": req.Priority,
		})
	} else {
		h.writeError(w, http.StatusNotFound, "Book not found or cannot update priority")
	}
}

// handleReorderQueue handles bulk queue reordering
// POST /api/queue/reorder
func (h *Handler) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	var req map[string]int

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.logger.Info("Reorder queue request", zap.Int("count", len(req)))

	success := h.backend.ReorderQueue(req)
	
	if success {
		h.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "success",
			"message": "Queue reordered",
		})
	} else {
		h.writeError(w, http.StatusInternalServerError, "Failed to reorder queue")
	}
}

// handleQueueOrder handles queue order requests
// GET /api/queue/order
func (h *Handler) handleQueueOrder(w http.ResponseWriter, r *http.Request) {
	order := h.backend.GetQueueOrder()
	
	h.logger.Info("Queue order request")

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"queue": order,
	})
}

// handleActiveDownloads handles active downloads list
// GET /api/downloads/active
func (h *Handler) handleActiveDownloads(w http.ResponseWriter, r *http.Request) {
	activeDownloads := h.backend.GetActiveDownloads()
	
	h.logger.Info("Active downloads request")

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"active_downloads": activeDownloads,
	})
}

// handleClearCompleted handles clearing completed downloads
// DELETE /api/queue/clear
func (h *Handler) handleClearCompleted(w http.ResponseWriter, r *http.Request) {
	count := h.backend.ClearCompleted()
	
	h.logger.Info("Clear completed request", zap.Int("cleared", count))

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"message": "Completed items cleared",
		"count": count,
	})
}
