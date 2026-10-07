package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"idmm/internal/engine"
	"idmm/internal/media"
	"idmm/internal/scheduler"
	"idmm/internal/torrent"
)

// TaskEntry wraps an active download task with its metadata.
type TaskEntry struct {
	ID                string             `json:"id"`
	URL               string             `json:"url"`
	Filename          string             `json:"filename"`
	TargetPath        string             `json:"target_path"`
	TotalSize         int64              `json:"total_size"`
	DownloadedBytes   int64              `json:"downloaded_bytes"`
	SpeedBytesPerSec  int64              `json:"speed_bytes_sec"`
	ETASeconds        int64              `json:"eta_seconds"`
	ProgressPercent   float64            `json:"progress_percent"`
	ActiveConnections int                `json:"active_connections"`
	Status            engine.TaskStatus  `json:"status"`
	ScheduledAt       *time.Time         `json:"scheduled_at,omitempty"`
	Segments          []*engine.Segment  `json:"segments"`
	Downloader        *engine.Downloader `json:"-"`
	Cancel            context.CancelFunc `json:"-"`
	CreatedAt         time.Time          `json:"created_at"`
}

// Server provides REST APIs, SSE, Stream Proxy, and Desktop Web UI.
type Server struct {
	mu            sync.RWMutex
	tasks         map[string]*TaskEntry
	streamServer  *media.StreamServer
	scheduler     *scheduler.Scheduler
	torrentClient *torrent.Client
	port          int
	httpServer    *http.Server
	webDir        string
	nextID        int
}

// NewServer initializes the backend server.
func NewServer(webDir string, preferredPort int) *Server {
	return &Server{
		tasks:        make(map[string]*TaskEntry),
		streamServer: media.NewStreamServer(0),
		scheduler:    scheduler.NewScheduler(),
		port:         preferredPort,
		webDir:       webDir,
		nextID:       1,
	}
}

// Start spins up the server on 127.0.0.1.
func (s *Server) Start() (string, error) {
	_, err := s.streamServer.Start()
	if err != nil {
		return "", fmt.Errorf("stream server failed: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("failed to bind server port: %w", err)
		}
	}

	s.port = ln.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", s.port)

	mux := http.NewServeMux()

	// REST APIs
	mux.HandleFunc("/api/tasks", s.handleTasks)
	mux.HandleFunc("/api/tasks/upload-torrent", s.handleUploadTorrent)
	mux.HandleFunc("/api/tasks/", s.handleTaskAction)
	mux.HandleFunc("/api/events", s.handleEvents)

	// Stream Proxy
	mux.HandleFunc("/stream/", func(w http.ResponseWriter, r *http.Request) {
		s.handleProxyStream(w, r)
	})

	// Static Web Assets
	fs := http.FileServer(http.Dir(s.webDir))
	mux.Handle("/", fs)

	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = s.httpServer.Serve(ln)
	}()

	return baseURL, nil
}

// LaunchAppMode launches Microsoft Edge or Chrome in seamless native desktop app mode.
func (s *Server) LaunchAppMode(url string) error {
	if runtime.GOOS == "windows" {
		edgePaths := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		}

		for _, p := range edgePaths {
			if _, err := os.Stat(p); err == nil {
				cmd := exec.Command(p, fmt.Sprintf("--app=%s", url), "--window-size=1150,780")
				return cmd.Start()
			}
		}

		cmd := exec.Command("cmd", "/c", "start", url)
		return cmd.Start()
	}

	return nil
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodGet {
		s.mu.RLock()
		list := make([]*TaskEntry, 0, len(s.tasks))
		for _, t := range s.tasks {
			list = append(list, t)
		}
		s.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(list)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			URL          string `json:"url"`
			Connections  int    `json:"connections"`
			SpeedLimitKB int64  `json:"speed_limit_kb"`
			TargetPath   string `json:"target_path"`
			ScheduledAt  string `json:"scheduled_at,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.URL == "" {
			http.Error(w, "URL is required", http.StatusBadRequest)
			return
		}

		var schedTime *time.Time
		if req.ScheduledAt != "" {
			// Try RFC3339 or ISO format
			t, err := time.Parse(time.RFC3339, req.ScheduledAt)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04", req.ScheduledAt)
			}
			if err == nil && t.After(time.Now()) {
				schedTime = &t
			}
		}

		entry, err := s.createAndStartTask(req.URL, req.TargetPath, req.Connections, req.SpeedLimitKB, schedTime)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(entry)
		return
	}

	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func (s *Server) createAndStartTask(url, targetPath string, conns int, speedLimitKB int64, schedTime *time.Time) (*TaskEntry, error) {
	s.mu.Lock()
	taskID := strconv.Itoa(s.nextID)
	s.nextID++
	s.mu.Unlock()

	if isTorrentInput(url) {
		return s.createAndStartTorrentTask(taskID, url, targetPath, schedTime)
	}

	cfg := engine.DefaultConfig(url, targetPath)
	if conns > 0 {
		cfg.Connections = conns
	}
	if speedLimitKB > 0 {
		cfg.MaxSpeedBytesPerSec = speedLimitKB * 1024
	}

	downloader := engine.NewDownloader(cfg)

	// Probe
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer probeCancel()
	info, err := downloader.Probe(probeCtx)
	if err != nil {
		return nil, fmt.Errorf("probe failed: %w", err)
	}

	initialStatus := engine.StatusDownloading
	if schedTime != nil {
		initialStatus = engine.StatusScheduled
	}

	entry := &TaskEntry{
		ID:                taskID,
		URL:               url,
		Filename:          info.Filename,
		TargetPath:        downloader.TargetPath(),
		TotalSize:         info.TotalSize,
		Status:            initialStatus,
		ScheduledAt:       schedTime,
		Downloader:        downloader,
		CreatedAt:         time.Now(),
		ActiveConnections: cfg.Connections,
	}

	s.mu.Lock()
	s.tasks[taskID] = entry
	s.mu.Unlock()

	s.streamServer.RegisterTask(taskID, downloader)

	runDownload := func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		entry.Cancel = cancel
		entry.Status = engine.StatusDownloading
		s.mu.Unlock()

		go func() {
			for update := range downloader.ProgressChannel() {
				s.mu.Lock()
				entry.DownloadedBytes = update.DownloadedBytes
				entry.SpeedBytesPerSec = update.SpeedBytesPerSec
				entry.ETASeconds = update.ETASeconds
				entry.ProgressPercent = update.ProgressPercent
				entry.ActiveConnections = update.ActiveConnections
				entry.Segments = update.Segments
				entry.Status = update.Status
				s.mu.Unlock()
			}
		}()

		_ = downloader.Start(ctx)

		s.mu.Lock()
		entry.Status = downloader.Status()
		s.mu.Unlock()
	}

	if schedTime != nil {
		_, err := s.scheduler.Schedule(taskID, *schedTime, runDownload)
		if err != nil {
			// Fallback: run immediately if scheduling fails
			go runDownload()
		}
	} else {
		go runDownload()
	}

	return entry, nil
}

func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Task ID missing", http.StatusBadRequest)
		return
	}
	taskID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	s.mu.RLock()
	entry, exists := s.tasks[taskID]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	if r.Method == http.MethodDelete {
		s.scheduler.Cancel(taskID)
		if entry.Cancel != nil {
			entry.Cancel()
		}
		s.mu.Lock()
		delete(s.tasks, taskID)
		s.mu.Unlock()
		s.streamServer.UnregisterTask(taskID)
		w.WriteHeader(http.StatusOK)
		return
	}

	switch action {
	case "pause":
		s.scheduler.Cancel(taskID)
		if entry.Downloader != nil {
			entry.Downloader.Pause()
		}
		s.mu.Lock()
		entry.Status = engine.StatusPaused
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case "resume":
		if entry.Status == engine.StatusPaused || entry.Status == engine.StatusScheduled {
			ctx, cancel := context.WithCancel(context.Background())
			s.mu.Lock()
			entry.Cancel = cancel
			entry.Status = engine.StatusDownloading
			s.mu.Unlock()
			go func() {
				_ = entry.Downloader.Start(ctx)
			}()
		}
		w.WriteHeader(http.StatusOK)
	case "open-folder":
		if runtime.GOOS == "windows" {
			absPath, _ := filepath.Abs(entry.TargetPath)
			_ = exec.Command("explorer.exe", "/select,", absPath).Start()
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			list := make([]*TaskEntry, 0, len(s.tasks))
			for _, t := range s.tasks {
				list = append(list, t)
			}
			s.mu.RUnlock()

			data, err := json.Marshal(list)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleProxyStream(w http.ResponseWriter, r *http.Request) {
	s.streamServer.ServeHTTP(w, r)
}

func isTorrentInput(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if strings.HasPrefix(lower, "magnet:?") {
		return true
	}
	if strings.HasSuffix(lower, ".torrent") {
		return true
	}
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(info.Name()), ".torrent") {
			return true
		}
	}
	return false
}

func (s *Server) createAndStartTorrentTask(taskID, url, targetPath string, schedTime *time.Time) (*TaskEntry, error) {
	s.mu.Lock()
	if s.torrentClient == nil {
		tc, err := torrent.NewClient(torrent.DefaultClientConfig("./downloads"))
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to init torrent client: %w", err)
		}
		s.torrentClient = tc
	}
	client := s.torrentClient
	s.mu.Unlock()

	dl, err := client.AddInput(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed adding torrent input: %w", err)
	}

	initialStatus := engine.StatusDownloading
	if schedTime != nil {
		initialStatus = engine.StatusScheduled
	}

	name := "Torrent (" + dl.InfoHash()[:8] + ")"
	if dl.HasMetadata() {
		name = dl.Name()
	}

	entry := &TaskEntry{
		ID:                taskID,
		URL:               url,
		Filename:          name,
		TargetPath:        dl.SavePath(),
		TotalSize:         dl.TotalLength(),
		Status:            initialStatus,
		ScheduledAt:       schedTime,
		CreatedAt:         time.Now(),
		ActiveConnections: 0,
	}

	s.mu.Lock()
	s.tasks[taskID] = entry
	s.mu.Unlock()

	runTorrent := func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		entry.Cancel = cancel
		entry.Status = engine.StatusDownloading
		s.mu.Unlock()

		go func() {
			if !dl.HasMetadata() {
				if err := dl.WaitForMetadata(ctx); err != nil {
					return
				}
				s.mu.Lock()
				entry.Filename = dl.Name()
				entry.TotalSize = dl.TotalLength()
				entry.TargetPath = dl.SavePath()
				s.mu.Unlock()
			}
			dl.Start()

			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					stats := dl.Stats()
					s.mu.Lock()
					entry.DownloadedBytes = stats.CompletedBytes
					entry.TotalSize = stats.TotalBytes
					entry.SpeedBytesPerSec = stats.DownloadSpeed
					entry.ProgressPercent = stats.Progress
					entry.ActiveConnections = stats.ConnectedPeers
					if stats.Progress >= 100.0 || dl.IsComplete() {
						entry.Status = engine.StatusCompleted
					}
					s.mu.Unlock()

					if stats.Progress >= 100.0 || dl.IsComplete() {
						return
					}
				}
			}
		}()
	}

	if schedTime != nil {
		_, _ = s.scheduler.Schedule(taskID, *schedTime, runTorrent)
	} else {
		runTorrent()
	}

	return entry, nil
}

func (s *Server) handleUploadTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 32MB max
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("torrent")
	if err != nil {
		http.Error(w, "Missing 'torrent' file field: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	targetPath := r.FormValue("target_path")
	entry, err := s.createAndStartTorrentBytesTask(data, header.Filename, targetPath)
	if err != nil {
		http.Error(w, "Failed adding torrent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (s *Server) createAndStartTorrentBytesTask(data []byte, originalFilename, targetPath string) (*TaskEntry, error) {
	s.mu.Lock()
	taskID := strconv.Itoa(s.nextID)
	s.nextID++

	if s.torrentClient == nil {
		dataDir := "./downloads"
		if targetPath != "" {
			dataDir = targetPath
		}
		tc, err := torrent.NewClient(torrent.DefaultClientConfig(dataDir))
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to init torrent client: %w", err)
		}
		s.torrentClient = tc
	}
	client := s.torrentClient
	s.mu.Unlock()

	dl, err := client.AddTorrentBytes(context.Background(), data)
	if err != nil {
		return nil, fmt.Errorf("failed adding torrent bytes: %w", err)
	}

	name := originalFilename
	if dl.HasMetadata() {
		name = dl.Name()
	}

	entry := &TaskEntry{
		ID:                taskID,
		URL:               "file://" + originalFilename,
		Filename:          name,
		TargetPath:        dl.SavePath(),
		TotalSize:         dl.TotalLength(),
		Status:            engine.StatusDownloading,
		CreatedAt:         time.Now(),
		ActiveConnections: 0,
	}

	s.mu.Lock()
	s.tasks[taskID] = entry
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	entry.Cancel = cancel
	s.mu.Unlock()

	go func() {
		if !dl.HasMetadata() {
			if err := dl.WaitForMetadata(ctx); err != nil {
				return
			}
			s.mu.Lock()
			entry.Filename = dl.Name()
			entry.TotalSize = dl.TotalLength()
			entry.TargetPath = dl.SavePath()
			s.mu.Unlock()
		}
		dl.Start()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := dl.Stats()
				s.mu.Lock()
				entry.DownloadedBytes = stats.CompletedBytes
				entry.TotalSize = stats.TotalBytes
				entry.SpeedBytesPerSec = stats.DownloadSpeed
				entry.ProgressPercent = stats.Progress
				entry.ActiveConnections = stats.ConnectedPeers
				if stats.Progress >= 100.0 || dl.IsComplete() {
					entry.Status = engine.StatusCompleted
				}
				s.mu.Unlock()

				if stats.Progress >= 100.0 || dl.IsComplete() {
					return
				}
			}
		}
	}()

	return entry, nil
}

