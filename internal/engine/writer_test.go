package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileWriterDirectAccess(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "direct_test.bin")
	var totalSize int64 = 1024 * 1024 // 1 MB

	fw, err := OpenFileWriter(targetPath, totalSize)
	if err != nil {
		t.Fatalf("OpenFileWriter failed: %v", err)
	}
	defer fw.Close()

	// Verify pre-allocation
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("os.Stat failed: %v", err)
	}
	if info.Size() < totalSize {
		t.Errorf("expected pre-allocated size >= %d, got %d", totalSize, info.Size())
	}

	// Concurrently write chunks at different offsets without overlapping
	var wg sync.WaitGroup
	chunks := 4
	chunkSize := totalSize / int64(chunks)

	for i := 0; i < chunks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			offset := int64(idx) * chunkSize
			payload := bytes.Repeat([]byte{byte(idx + 1)}, int(chunkSize))
			n, writeErr := fw.WriteAt(payload, offset)
			if writeErr != nil {
				t.Errorf("worker %d WriteAt failed: %v", idx, writeErr)
			}
			if int64(n) != chunkSize {
				t.Errorf("worker %d wrote %d bytes, expected %d", idx, n, chunkSize)
			}
		}(i)
	}

	wg.Wait()
	_ = fw.Sync()

	// Read back and verify each chunk
	readData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	for i := 0; i < chunks; i++ {
		offset := int64(i) * chunkSize
		expectedByte := byte(i + 1)
		sub := readData[offset : offset+chunkSize]
		for j, b := range sub {
			if b != expectedByte {
				t.Fatalf("mismatch at chunk %d byte %d: expected %d, got %d", i, j, expectedByte, b)
			}
		}
	}
}
