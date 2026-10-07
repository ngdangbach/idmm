package nativemsg

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadWriteMessage(t *testing.T) {
	var buf bytes.Buffer

	original := &Message{
		Action:      "download",
		URL:         "https://example.com/test.iso",
		Connections: 16,
	}

	resp := &Response{
		Status:  "ok",
		Message: "all good",
		TaskID:  "42",
	}

	// Test WriteResponse
	if err := WriteResponse(&buf, resp); err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Test ReadMessage format on another message
	var msgBuf bytes.Buffer
	testMsg := &Message{Action: "ping"}
	data := []byte(`{"action":"ping"}`)
	length := uint32(len(data))
	_ = binaryWrite(&msgBuf, length)
	msgBuf.Write(data)

	readMsg, err := ReadMessage(&msgBuf)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if readMsg.Action != testMsg.Action {
		t.Errorf("expected action %s, got %s", testMsg.Action, readMsg.Action)
	}
	_ = original
}

func TestHostPing(t *testing.T) {
	host := NewHost("http://127.0.0.1:8989")
	resp := host.Handle(&Message{Action: "ping"})
	if resp.Status != "ok" || resp.Message != "pong" {
		t.Errorf("expected ok/pong, got %v", resp)
	}
}

func TestHostDownloadForwarding(t *testing.T) {
	// Mock IDMM server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tasks" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"task-999"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	host := NewHost(server.URL)
	resp := host.Handle(&Message{
		Action: "download",
		URL:    "https://example.com/video.mp4",
	})

	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %v (%s)", resp.Status, resp.Message)
	}
	if resp.TaskID != "task-999" {
		t.Errorf("expected task_id task-999, got %s", resp.TaskID)
	}
}

func binaryWrite(buf *bytes.Buffer, v uint32) error {
	b := []byte{
		byte(v),
		byte(v >> 8),
		byte(v >> 16),
		byte(v >> 24),
	}
	_, err := buf.Write(b)
	return err
}
