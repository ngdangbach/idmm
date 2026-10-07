package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TaskState holds persistent snapshot data to enable resuming downloads.
type TaskState struct {
	TaskID       string     `json:"task_id"`
	URL          string     `json:"url"`
	TargetPath   string     `json:"target_path"`
	TotalSize    int64      `json:"total_size"`
	AcceptRanges bool       `json:"accept_ranges"`
	ETag         string     `json:"etag,omitempty"`
	LastModified string     `json:"last_modified,omitempty"`
	Segments     []*Segment `json:"segments"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// StateFilePath returns the path to the sidecar journal file.
func StateFilePath(targetPath string) string {
	return targetPath + ".idmm.json"
}

// SaveState atomically writes task progress to disk.
func SaveState(targetPath string, state *TaskState) error {
	state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	stateFile := StateFilePath(targetPath)
	tempFile := fmt.Sprintf("%s.tmp.%d", stateFile, time.Now().UnixNano())

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic replace
	_ = os.Remove(stateFile)
	if err := os.Rename(tempFile, stateFile); err != nil {
		return fmt.Errorf("failed to commit state file: %w", err)
	}
	return nil
}

// LoadState reads task state if a journal file exists.
func LoadState(targetPath string) (*TaskState, error) {
	stateFile := StateFilePath(targetPath)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}

	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", stateFile, err)
	}

	// Verify that destination file also exists
	if _, err := os.Stat(state.TargetPath); err != nil {
		return nil, fmt.Errorf("target file missing, cannot resume: %w", err)
	}

	return &state, nil
}

// RemoveState removes the journal file upon successful completion.
func RemoveState(targetPath string) {
	stateFile := StateFilePath(targetPath)
	_ = os.Remove(stateFile)
}
