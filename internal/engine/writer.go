package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileWriter handles concurrent random-access writes directly into the target destination file.
// It eliminates the need for temporary chunks and file-stitching after downloads.
type FileWriter struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	size     int64
	isDirect bool
}

// OpenFileWriter creates or opens the destination file and pre-allocates disk space if size > 0.
func OpenFileWriter(targetPath string, totalSize int64) (*FileWriter, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Open or create file for reading and writing without truncation
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open destination file %s: %w", targetPath, err)
	}

	fw := &FileWriter{
		file:     file,
		path:     targetPath,
		size:     totalSize,
		isDirect: totalSize > 0,
	}

	// Pre-allocate space on disk (Sparse / Truncate)
	if totalSize > 0 {
		currentStat, err := file.Stat()
		if err == nil && currentStat.Size() < totalSize {
			if err := file.Truncate(totalSize); err != nil {
				_ = err
			}
		}
	}

	return fw, nil
}

// WriteAt writes data at the specified byte offset concurrently.
func (fw *FileWriter) WriteAt(p []byte, offset int64) (int, error) {
	return fw.file.WriteAt(p, offset)
}

// ReadAt reads data at the specified byte offset concurrently (used by streaming proxy).
func (fw *FileWriter) ReadAt(p []byte, offset int64) (int, error) {
	fw.mu.Lock()
	file := fw.file
	fw.mu.Unlock()
	if file == nil {
		return 0, os.ErrClosed
	}
	return file.ReadAt(p, offset)
}

// Write appends data sequentially (used when server does not support Range / chunked stream).
func (fw *FileWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.file.Write(p)
}

// Sync flushes cached writes to persistent storage.
func (fw *FileWriter) Sync() error {
	return fw.file.Sync()
}

// Close flushes and closes the file.
func (fw *FileWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.file != nil {
		_ = fw.file.Sync()
		err := fw.file.Close()
		fw.file = nil
		return err
	}
	return nil
}

// FilePath returns the target file path.
func (fw *FileWriter) FilePath() string {
	return fw.path
}
