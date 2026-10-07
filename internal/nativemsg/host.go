package nativemsg

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Message is the generic JSON payload exchanged with the browser extension.
type Message struct {
	Action      string            `json:"action"`
	URL         string            `json:"url,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	Connections int               `json:"connections,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Referer     string            `json:"referer,omitempty"`
	Cookies     string            `json:"cookies,omitempty"`
}

// Response is the response sent back to the browser.
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	TaskID  string      `json:"task_id,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Host handles communication with the Chrome/Edge Extension.
type Host struct {
	serverURL string
	client    *http.Client
}

// NewHost creates a new native messaging host.
func NewHost(serverURL string) *Host {
	if serverURL == "" {
		serverURL = "http://127.0.0.1:8989"
	}
	return &Host{
		serverURL: serverURL,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

// ReadMessage reads a single length-prefixed message from reader.
func ReadMessage(r io.Reader) (*Message, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}

	if length == 0 || length > 10*1024*1024 { // Cap at 10MB
		return nil, fmt.Errorf("invalid message length: %d", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("failed reading full message payload: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("failed parsing JSON payload: %w", err)
	}

	return &msg, nil
}

// WriteResponse writes a length-prefixed JSON response to writer.
func WriteResponse(w io.Writer, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	length := uint32(len(data))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// Handle processes incoming message and calls backend REST API.
func (h *Host) Handle(msg *Message) *Response {
	switch msg.Action {
	case "ping":
		return &Response{
			Status:  "ok",
			Message: "pong",
			Data:    map[string]string{"version": "1.0.0", "engine": "idmm-go"},
		}

	case "download":
		if msg.URL == "" {
			return &Response{Status: "error", Message: "URL is required"}
		}

		conns := msg.Connections
		if conns <= 0 {
			conns = 16
		}

		reqBody, _ := json.Marshal(map[string]interface{}{
			"url":         msg.URL,
			"connections": conns,
			"target_path": msg.Filename,
		})

		resp, err := h.client.Post(h.serverURL+"/api/tasks", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			return &Response{
				Status:  "error",
				Message: fmt.Sprintf("IDMM app not reachable at %s. Please make sure IDMM is running.", h.serverURL),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return &Response{Status: "error", Message: string(body)}
		}

		var created map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&created)

		taskID := ""
		if idVal, ok := created["id"].(string); ok {
			taskID = idVal
		}

		return &Response{
			Status:  "ok",
			Message: "Download intercepted and dispatched to IDMM",
			TaskID:  taskID,
		}

	case "status":
		resp, err := h.client.Get(h.serverURL + "/api/tasks")
		if err != nil {
			return &Response{Status: "error", Message: "IDMM server unavailable"}
		}
		defer resp.Body.Close()

		var tasks []interface{}
		_ = json.NewDecoder(resp.Body).Decode(&tasks)

		return &Response{
			Status: "ok",
			Data:   tasks,
		}

	default:
		return &Response{Status: "error", Message: fmt.Sprintf("unknown action: %s", msg.Action)}
	}
}

// RunLoop runs the standard I/O communication loop.
func (h *Host) RunLoop() {
	for {
		msg, err := ReadMessage(os.Stdin)
		if err != nil {
			if err == io.EOF {
				return
			}
			_ = WriteResponse(os.Stdout, &Response{Status: "error", Message: err.Error()})
			return
		}

		resp := h.Handle(msg)
		if err := WriteResponse(os.Stdout, resp); err != nil {
			return
		}
	}
}
