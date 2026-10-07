package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
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

	// Cancel task to release open file handle on Windows
	srv.mu.Lock()
	for _, entry := range srv.tasks {
		if entry.Cancel != nil {
			entry.Cancel()
		}
	}
	srv.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
}

func TestServerTorrentTasks(t *testing.T) {
	tempDir := t.TempDir()
	webDir := filepath.Join(tempDir, "web")
	_ = os.MkdirAll(webDir, 0755)
	_ = os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<h1>IDMM</h1>"), 0644)

	srv := NewServer(webDir, 0)
	baseURL, err := srv.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// 1. Test POST /api/tasks with Magnet link
	magnetURI := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=Ubuntu+ISO"
	reqBody, _ := json.Marshal(map[string]interface{}{
		"url": magnetURI,
	})
	postResp, err := http.Post(baseURL+"/api/tasks", "application/json", bytes.NewReader(reqBody))
	if err != nil || postResp.StatusCode != http.StatusOK {
		t.Fatalf("failed to create magnet task: %v, code: %d", err, postResp.StatusCode)
	}
	var magnetTask TaskEntry
	_ = json.NewDecoder(postResp.Body).Decode(&magnetTask)
	_ = postResp.Body.Close()

	if magnetTask.ID == "" {
		t.Fatal("expected non-empty ID for magnet task")
	}
	if magnetTask.Filename != "Torrent (01234567)" {
		t.Errorf("unexpected magnet task filename: %s", magnetTask.Filename)
	}

	// 2. Test POST /api/tasks/upload-torrent with multipart .torrent file upload
	sampleFile := filepath.Join(tempDir, "sample.txt")
	_ = os.WriteFile(sampleFile, []byte("IDMM Test Torrent Content for Server Upload"), 0644)

	info := metainfo.Info{PieceLength: 32 * 1024}
	if err := info.BuildFromFilePath(sampleFile); err != nil {
		t.Fatalf("BuildFromFilePath failed: %v", err)
	}
	infoBytes, _ := bencode.Marshal(info)
	mi := metainfo.MetaInfo{InfoBytes: infoBytes}
	mi.SetDefaults()

	var torrentBuf bytes.Buffer
	if err := mi.Write(&torrentBuf); err != nil {
		t.Fatalf("failed writing metainfo: %v", err)
	}

	// Build multipart form
	var body bytes.Buffer
	mpWriter := multipart.NewWriter(&body)
	fileWriter, err := mpWriter.CreateFormFile("torrent", "sample.txt.torrent")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_, _ = fileWriter.Write(torrentBuf.Bytes())
	_ = mpWriter.Close()

	uploadResp, err := http.Post(baseURL+"/api/tasks/upload-torrent", mpWriter.FormDataContentType(), &body)
	if err != nil || uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("upload torrent failed: %v, status: %d", err, uploadResp.StatusCode)
	}
	var uploadTask TaskEntry
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadTask)
	_ = uploadResp.Body.Close()

	if uploadTask.ID == "" {
		t.Fatal("expected non-empty ID for uploaded torrent task")
	}
	if uploadTask.Filename != "sample.txt" {
		t.Errorf("expected filename 'sample.txt', got '%s'", uploadTask.Filename)
	}

	// 3. Verify GET /api/tasks contains both tasks
	resp, err := http.Get(baseURL + "/api/tasks")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to get tasks: %v", err)
	}
	var tasks []*TaskEntry
	_ = json.NewDecoder(resp.Body).Decode(&tasks)
	_ = resp.Body.Close()

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks in list, got %d", len(tasks))
	}

	srv.mu.Lock()
	for _, entry := range srv.tasks {
		if entry.Cancel != nil {
			entry.Cancel()
		}
	}
	if srv.torrentClient != nil {
		_ = srv.torrentClient.Close()
	}
	srv.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
}
