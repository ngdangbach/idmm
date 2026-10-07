package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServerTaskEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	webDir := filepath.Join(tempDir, "web")
	_ = os.MkdirAll(webDir, 0755)
	_ = os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<h1>IDMM</h1>"), 0644)

	// Mock remote file server
	mockContent := []byte("HELLO_WORLD_IDMM_SERVER_TEST_PAYLOAD")
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(mockContent)
	}))
	defer mockServer.Close()

	srv := NewServer(webDir, 0)
	baseURL, err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// 1. Test GET / (serves index.html)
	resp, err := http.Get(baseURL + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to get index.html: %v, status: %d", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. Test GET /api/tasks (initially empty)
	resp, err = http.Get(baseURL + "/api/tasks")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to get tasks: %v", err)
	}
	var tasks []*TaskEntry
	_ = json.NewDecoder(resp.Body).Decode(&tasks)
	_ = resp.Body.Close()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// 3. Test POST /api/tasks (create new task)
	reqBody, _ := json.Marshal(map[string]interface{}{
		"url":         mockServer.URL + "/testfile.bin",
		"connections": 4,
		"target_path": filepath.Join(tempDir, "downloaded.bin"),
	})
	postResp, err := http.Post(baseURL+"/api/tasks", "application/json", bytes.NewReader(reqBody))
	if err != nil || postResp.StatusCode != http.StatusOK {
		t.Fatalf("failed to create task: %v, code: %d", err, postResp.StatusCode)
	}
	var created TaskEntry
	_ = json.NewDecoder(postResp.Body).Decode(&created)
	_ = postResp.Body.Close()

	if created.ID == "" {
		t.Fatal("expected created task to have non-empty ID")
	}

	// 4. Test GET /api/tasks again
	resp, err = http.Get(baseURL + "/api/tasks")
	if err != nil {
		t.Fatalf("failed to get tasks: %v", err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&tasks)
	_ = resp.Body.Close()

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != created.ID {
		t.Errorf("expected task ID %s, got %s", created.ID, tasks[0].ID)
	}
}
