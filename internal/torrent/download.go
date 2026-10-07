package torrent

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// DownloadStatus represents the state of a torrent download.
type DownloadStatus string

const (
	StatusFetchingMetadata DownloadStatus = "METADATA"
	StatusDownloading      DownloadStatus = "DOWNLOADING"
	StatusSeeding          DownloadStatus = "SEEDING"
	StatusCompleted        DownloadStatus = "COMPLETED"
	StatusPaused           DownloadStatus = "PAUSED"
	StatusError            DownloadStatus = "ERROR"
)

// FileInfo contains metadata for an individual file within the torrent.
type FileInfo struct {
	Index          int    `json:"index"`
	Path           string `json:"path"`
	Length         int64  `json:"length"`
	BytesCompleted int64  `json:"bytes_completed"`
	Progress       float64 `json:"progress"`
}

// DownloadStats contains real-time progress metrics.
type DownloadStats struct {
	InfoHash       string         `json:"info_hash"`
	Name           string         `json:"name"`
	Status         DownloadStatus `json:"status"`
	TotalBytes     int64          `json:"total_bytes"`
	CompletedBytes int64          `json:"completed_bytes"`
	Progress       float64        `json:"progress"`
	DownloadSpeed  int64          `json:"download_speed_bps"` // bytes per second
	UploadSpeed    int64          `json:"upload_speed_bps"`
	ConnectedPeers int            `json:"connected_peers"`
	ActivePieces   int            `json:"active_pieces"`
	TotalPieces    int            `json:"total_pieces"`
	ETA            string         `json:"eta"`
}

// Download represents an active torrent download session.
type Download struct {
	t        *torrent.Torrent
	dataDir  string
	mu       sync.RWMutex

	lastBytes    int64
	lastChecked  time.Time
	currentSpeed int64
	status       DownloadStatus
}

func newDownload(t *torrent.Torrent, dataDir string) *Download {
	return &Download{
		t:           t,
		dataDir:     dataDir,
		lastChecked: time.Now(),
		status:      StatusFetchingMetadata,
	}
}

// InfoHash returns the hex representation of the torrent infohash.
func (d *Download) InfoHash() string {
	return d.t.InfoHash().HexString()
}

// WaitForMetadata blocks until the torrent metadata has been downloaded or context expires.
func (d *Download) WaitForMetadata(ctx context.Context) error {
	select {
	case <-d.t.GotInfo():
		d.mu.Lock()
		d.status = StatusDownloading
		d.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HasMetadata checks if torrent metadata is already available.
func (d *Download) HasMetadata() bool {
	select {
	case <-d.t.GotInfo():
		return true
	default:
		return false
	}
}

// Start begins downloading all pieces of the torrent.
func (d *Download) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.HasMetadata() {
		return
	}
	d.t.DownloadAll()
	d.status = StatusDownloading
}

// Name returns the torrent or top-level folder name.
func (d *Download) Name() string {
	if d.HasMetadata() {
		return d.t.Name()
	}
	return "Fetching metadata..."
}

// TotalLength returns total bytes of all files in the torrent.
func (d *Download) TotalLength() int64 {
	if d.HasMetadata() {
		return d.t.Length()
	}
	return 0
}

// BytesCompleted returns number of verified bytes stored on disk.
func (d *Download) BytesCompleted() int64 {
	return d.t.BytesCompleted()
}

// Files returns the list of files included in the torrent.
func (d *Download) Files() []FileInfo {
	if !d.HasMetadata() {
		return nil
	}

	rawFiles := d.t.Files()
	result := make([]FileInfo, len(rawFiles))
	for i, f := range rawFiles {
		completed := f.BytesCompleted()
		length := f.Length()
		var progress float64
		if length > 0 {
			progress = float64(completed) / float64(length) * 100
		}
		result[i] = FileInfo{
			Index:          i,
			Path:           f.Path(),
			Length:         length,
			BytesCompleted: completed,
			Progress:       progress,
		}
	}
	return result
}

// NewStreamReader returns a seekable reader for streaming a specific file while downloading.
func (d *Download) NewStreamReader(fileIndex int) (torrent.Reader, error) {
	if !d.HasMetadata() {
		return nil, fmt.Errorf("metadata not available yet")
	}

	files := d.t.Files()
	if fileIndex < 0 || fileIndex >= len(files) {
		return nil, fmt.Errorf("invalid file index: %d", fileIndex)
	}

	f := files[fileIndex]
	// Prioritize this file
	f.Download()
	reader := f.NewReader()
	// Set sequential piece prioritization for smooth playback
	reader.SetReadahead(10 * 1024 * 1024) // 10MB readahead
	return reader, nil
}

// Stats returns a snapshot of current download metrics and computes transfer speed.
func (d *Download) Stats() DownloadStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	hasInfo := d.HasMetadata()
	var total int64
	var totalPieces int
	if hasInfo {
		total = d.t.Length()
		totalPieces = d.t.NumPieces()
	}

	completed := d.t.BytesCompleted()
	now := time.Now()
	elapsed := now.Sub(d.lastChecked).Seconds()

	if elapsed >= 0.5 {
		diff := completed - d.lastBytes
		if diff < 0 {
			diff = 0
		}
		d.currentSpeed = int64(float64(diff) / elapsed)
		d.lastBytes = completed
		d.lastChecked = now
	}

	var progress float64
	if total > 0 {
		progress = float64(completed) / float64(total) * 100
		if progress >= 100.0 || d.t.Complete().Bool() {
			progress = 100.0
			d.status = StatusCompleted
		}
	} else if d.t.Complete().Bool() {
		progress = 100.0
		d.status = StatusCompleted
	}

	// Compute ETA
	eta := "--:--"
	if d.currentSpeed > 0 && total > completed {
		remainingSec := (total - completed) / d.currentSpeed
		if remainingSec < 60 {
			eta = fmt.Sprintf("%ds", remainingSec)
		} else if remainingSec < 3600 {
			eta = fmt.Sprintf("%dm %ds", remainingSec/60, remainingSec%60)
		} else {
			eta = fmt.Sprintf("%dh %dm", remainingSec/3600, (remainingSec%3600)/60)
		}
	} else if progress >= 100.0 || d.t.Complete().Bool() {
		eta = "Done"
	}

	status := d.status
	if !hasInfo {
		status = StatusFetchingMetadata
	} else if progress >= 100.0 || d.t.Complete().Bool() {
		status = StatusCompleted
	}

	name := "Magnet / Torrent"
	if hasInfo {
		name = d.t.Name()
	}

	activePeers := len(d.t.PeerConns())

	return DownloadStats{
		InfoHash:       d.t.InfoHash().HexString(),
		Name:           name,
		Status:         status,
		TotalBytes:     total,
		CompletedBytes: completed,
		Progress:       progress,
		DownloadSpeed:  d.currentSpeed,
		ConnectedPeers: activePeers,
		TotalPieces:    totalPieces,
		ETA:            eta,
	}
}

// SavePath returns the absolute path to the downloaded file or folder.
func (d *Download) SavePath() string {
	if !d.HasMetadata() {
		return d.dataDir
	}
	return filepath.Join(d.dataDir, d.t.Name())
}

// Drop stops and removes the torrent from the client.
func (d *Download) Drop() {
	d.t.Drop()
}

// AddClientPeer connects this torrent to another local or test Client.
func (d *Download) AddClientPeer(other *Client) int {
	if d == nil || d.t == nil || other == nil || other.inner == nil {
		return 0
	}
	return d.t.AddClientPeer(other.inner)
}

// VerifyData forces validation of existing downloaded files against piece hashes.
func (d *Download) VerifyData() {
	if d != nil && d.t != nil {
		d.t.VerifyData()
	}
}

// IsComplete returns true if all pieces have been downloaded and verified.
func (d *Download) IsComplete() bool {
	if d == nil || d.t == nil {
		return false
	}
	return d.t.Complete().Bool()
}

// Inner returns the underlying anacrolix Torrent object.
func (d *Download) Inner() *torrent.Torrent {
	if d == nil {
		return nil
	}
	return d.t
}
