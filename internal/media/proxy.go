package media

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"idmm/internal/engine"
)

// StreamServer hosts a local HTTP streaming proxy for VLC, MPV, and web browsers.
type StreamServer struct {
	mu        sync.RWMutex
	listener  net.Listener
	server    *http.Server
	port      int
	tasks     map[string]*engine.Downloader
	baseURL   string
	closeChan chan struct{}
}

// NewStreamServer initializes a local streaming server.
func NewStreamServer(preferredPort int) *StreamServer {
	return &StreamServer{
		port:      preferredPort,
		tasks:     make(map[string]*engine.Downloader),
		closeChan: make(chan struct{}),
	}
}

// RegisterTask binds a task ID to an active Downloader instance.
func (s *StreamServer) RegisterTask(taskID string, d *engine.Downloader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = d
}

// UnregisterTask removes task registration.
func (s *StreamServer) UnregisterTask(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, taskID)
}

// Start begins listening on localhost.
func (s *StreamServer) Start() (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// If preferred port is occupied, let OS pick any open port
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("failed to bind stream server: %w", err)
		}
	}

	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.baseURL = fmt.Sprintf("http://127.0.0.1:%d", s.port)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", s.handleStream)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  0, // Long-lived streaming connection
		WriteTimeout: 0,
	}

	go func() {
		_ = s.server.Serve(ln)
	}()

	return s.baseURL, nil
}

// GetStreamURL constructs the full streaming URL for a given task.
func (s *StreamServer) GetStreamURL(taskID string) string {
	s.mu.RLock()
	d, ok := s.tasks[taskID]
	s.mu.RUnlock()

	fn := "video.mp4"
	if ok && d.Filename() != "" {
		fn = d.Filename()
	}
	return fmt.Sprintf("%s/stream/%s/%s", s.baseURL, taskID, fn)
}

// ServeHTTP implements http.Handler for StreamServer.
func (s *StreamServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handleStream(w, r)
}

// handleStream handles media player HTTP Range requests.
func (s *StreamServer) handleStream(w http.ResponseWriter, r *http.Request) {
	// Path pattern: /stream/{taskID} or /stream/{taskID}/{filename}
	trimmed := strings.TrimPrefix(r.URL.Path, "/stream/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Task ID missing", http.StatusBadRequest)
		return
	}
	taskID := parts[0]

	s.mu.RLock()
	d, exists := s.tasks[taskID]
	s.mu.RUnlock()

	if !exists || d == nil {
		http.Error(w, "Download task not found or not registered", http.StatusNotFound)
		return
	}

	totalSize := d.TotalSize()
	contentType := d.ContentType()
	if contentType == "" {
		ext := filepath.Ext(d.Filename())
		contentType = mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	// Always advertise Range support
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodHead {
		if totalSize > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" || totalSize <= 0 {
		// Full stream (HTTP 200)
		if totalSize > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
		}
		w.WriteHeader(http.StatusOK)

		streamReader, err := d.NewStreamReader(r.Context(), 0, totalSize-1)
		if err != nil {
			return
		}
		defer streamReader.Close()
		_, _ = io.Copy(w, streamReader)
		return
	}

	// Parse Range: bytes=start-end
	start, end, err := parseRange(rangeHeader, totalSize)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	contentLength := end - start + 1
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.WriteHeader(http.StatusPartialContent)

	streamReader, err := d.NewStreamReader(r.Context(), start, end)
	if err != nil {
		return
	}
	defer streamReader.Close()

	_, _ = io.Copy(w, streamReader)
}

func parseRange(rangeHeader string, totalSize int64) (int64, int64, error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range prefix")
	}

	spec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range spec")
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		// Suffix range: bytes=-500 (last 500 bytes)
		length, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || length <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix length")
		}
		start = totalSize - length
		if start < 0 {
			start = 0
		}
		end = totalSize - 1
		return start, end, nil
	}

	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= totalSize {
		return 0, 0, fmt.Errorf("invalid range start")
	}

	if parts[1] == "" {
		// Range: bytes=start- (all the way to end)
		end = totalSize - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, fmt.Errorf("invalid range end")
		}
		if end >= totalSize {
			end = totalSize - 1
		}
	}

	return start, end, nil
}

// Stop gracefully closes the server.
func (s *StreamServer) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	}
}
