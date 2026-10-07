package engine

import "time"

// TaskStatus represents the lifecycle state of a download task.
type TaskStatus string

const (
	StatusPending     TaskStatus = "PENDING"
	StatusScheduled   TaskStatus = "SCHEDULED"
	StatusDownloading TaskStatus = "DOWNLOADING"
	StatusPaused      TaskStatus = "PAUSED"
	StatusCompleted   TaskStatus = "COMPLETED"
	StatusFailed      TaskStatus = "FAILED"
	StatusCanceled    TaskStatus = "CANCELED"
)

// SegmentStatus represents the state of a single connection chunk.
type SegmentStatus string

const (
	SegmentPending     SegmentStatus = "PENDING"
	SegmentDownloading SegmentStatus = "DOWNLOADING"
	SegmentCompleted   SegmentStatus = "COMPLETED"
	SegmentFailed      SegmentStatus = "FAILED"
)

// Segment defines a byte range within the target file.
type Segment struct {
	ID         int           `json:"id"`
	Start      int64         `json:"start"`      // Initial byte offset
	End        int64         `json:"end"`        // Inclusive end offset
	Downloaded int64         `json:"downloaded"` // Bytes retrieved for this segment
	Status     SegmentStatus `json:"status"`
}

// Remaining returns unread bytes remaining in this segment.
func (s *Segment) Remaining() int64 {
	rem := (s.End - s.Start + 1) - s.Downloaded
	if rem < 0 {
		return 0
	}
	return rem
}

// CurrentOffset returns the target file write offset for the next incoming byte.
func (s *Segment) CurrentOffset() int64 {
	return s.Start + s.Downloaded
}

// ProgressUpdate contains real-time metrics for UI/CLI consumers.
type ProgressUpdate struct {
	TaskID            string      `json:"task_id"`
	TotalSize         int64       `json:"total_size"`
	DownloadedBytes   int64       `json:"downloaded_bytes"`
	SpeedBytesPerSec  int64       `json:"speed_bytes_per_sec"`
	ETASeconds        int64       `json:"eta_seconds"`
	ProgressPercent   float64     `json:"progress_percent"`
	ActiveConnections int         `json:"active_connections"`
	Segments          []*Segment  `json:"segments"`
	Status            TaskStatus  `json:"status"`
	Error             string      `json:"error,omitempty"`
}

// TaskConfig configures parameters for a download task.
type TaskConfig struct {
	URL                 string            `json:"url"`
	TargetPath          string            `json:"target_path"`
	Connections         int               `json:"connections"`
	MaxSpeedBytesPerSec int64             `json:"max_speed_bytes_sec"`
	Headers             map[string]string `json:"headers,omitempty"`
	MinSegmentSize      int64             `json:"min_segment_size"`
	Timeout             time.Duration     `json:"timeout"`
}

// DefaultConfig provides sensible defaults.
func DefaultConfig(url, targetPath string) TaskConfig {
	return TaskConfig{
		URL:                 url,
		TargetPath:          targetPath,
		Connections:         8,
		MaxSpeedBytesPerSec: 0,
		MinSegmentSize:      512 * 1024,
		Timeout:             30 * time.Second,
	}
}
