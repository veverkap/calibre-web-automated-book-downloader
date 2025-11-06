package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"go.uber.org/zap"
)

func setupTestHandler() *Handler {
	cfg := &config.Config{
		FlaskHost:     "0.0.0.0",
		FlaskPort:     8084,
		StatusTimeout: 3600,
	}
	logger, _ := zap.NewDevelopment()
	return NewHandler(cfg, logger)
}

func TestHandleStatus(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	
	handler.handleStatus(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleSearch(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/search?title=test&author=author1&author=author2", nil)
	w := httptest.NewRecorder()
	
	handler.handleSearch(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleQueueOrder(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/queue/order", nil)
	w := httptest.NewRecorder()
	
	handler.handleQueueOrder(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleActiveDownloads(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/downloads/active", nil)
	w := httptest.NewRecorder()
	
	handler.handleActiveDownloads(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleClearCompleted(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("DELETE", "/api/queue/clear", nil)
	w := httptest.NewRecorder()
	
	handler.handleClearCompleted(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleSetPriority(t *testing.T) {
	handler := setupTestHandler()
	
	// First, we need to set up chi router context
	r := chi.NewRouter()
	r.Put("/api/queue/{book_id}/priority", handler.handleSetPriority)
	
	body := strings.NewReader(`{"priority": 5}`)
	req := httptest.NewRequest("PUT", "/api/queue/test-book-123/priority", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	r.ServeHTTP(w, req)
	
	// The book doesn't exist, so we expect a 404
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleReorderQueue(t *testing.T) {
	handler := setupTestHandler()
	
	body := strings.NewReader(`{"book1": 1, "book2": 2}`)
	req := httptest.NewRequest("POST", "/api/queue/reorder", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	handler.handleReorderQueue(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["status"] != "success" {
		t.Errorf("Expected status 'success', got '%v'", response["status"])
	}
}

func TestHandleInfoMissingID(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/info", nil)
	w := httptest.NewRecorder()
	
	handler.handleInfo(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleDownloadMissingID(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/download", nil)
	w := httptest.NewRecorder()
	
	handler.handleDownload(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleDownloadWithPriority(t *testing.T) {
	handler := setupTestHandler()
	
	req := httptest.NewRequest("GET", "/api/download?id=test-book&priority=10", nil)
	w := httptest.NewRecorder()
	
	handler.handleDownload(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	
	if response["priority"] != float64(10) {
		t.Errorf("Expected priority 10, got %v", response["priority"])
	}
}
