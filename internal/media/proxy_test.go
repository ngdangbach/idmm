package media

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"idmm/internal/engine"
)

func TestStreamServerRangeRequests(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "video.mp4")
	totalSize := int64(100 * 1024) // 100 KB mock file

	// Create a dummy file with known byte pattern
	content := make([]byte, totalSize)
	for i := range content {
		content[i] = byte(i % 256)
	}
	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Setup mock downloader
	cfg := engine.DefaultConfig("http://mock.example.com/video.mp4", targetPath)
	downloader := engine.NewDownloader(cfg)

	// Start streaming server
	server := NewStreamServer(0) // Random port
	baseURL, err := server.Start()
	if err != nil {
		t.Fatalf("failed to start stream server: %v", err)
	}
	defer server.Stop()

	// Register task
	taskID := "task_test_1"
	server.RegisterTask(taskID, downloader)

	// Test health endpoint
	resp, err := http.Get(baseURL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v", err)
	}
	_ = resp.Body.Close()

	streamURL := server.GetStreamURL(taskID)
	if streamURL == "" {
		t.Fatal("expected non-empty stream URL")
	}
	fmt.Printf("Stream URL: %s\n", streamURL)
}

func TestParseRange(t *testing.T) {
	var total int64 = 1000

	// Normal range
	s, e, err := parseRange("bytes=0-499", total)
	if err != nil || s != 0 || e != 499 {
		t.Errorf("expected 0-499, got %d-%d, err=%v", s, e, err)
	}

	// Open end range
	s, e, err = parseRange("bytes=500-", total)
	if err != nil || s != 500 || e != 999 {
		t.Errorf("expected 500-999, got %d-%d, err=%v", s, e, err)
	}

	// Suffix range
	s, e, err = parseRange("bytes=-200", total)
	if err != nil || s != 800 || e != 999 {
		t.Errorf("expected 800-999, got %d-%d, err=%v", s, e, err)
	}
}
