package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaOrganizer_Subfolders(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "idmm_tg_test_*")
	if err != nil {
		t.Fatalf("Không thể tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	organizer := NewMediaOrganizer(tempDir)

	testDate := time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC)

	// 1. Kiểm tra lưu ảnh vào thư mục images
	photoPath, err := organizer.GetDestinationPath(testDate, MediaTypeImage, "photo_1001.jpg")
	if err != nil {
		t.Fatalf("Lỗi GetDestinationPath ảnh: %v", err)
	}

	expectedImageSubdir := filepath.Join(tempDir, "2026-10-07", "images")
	if !strings.HasPrefix(photoPath, expectedImageSubdir) {
		t.Errorf("Kỳ vọng đường dẫn ảnh bắt đầu bằng '%s', nhận được '%s'", expectedImageSubdir, photoPath)
	}

	// 2. Kiểm tra lưu video vào thư mục videos
	videoPath, err := organizer.GetDestinationPath(testDate, MediaTypeVideo, "video_2002.mp4")
	if err != nil {
		t.Fatalf("Lỗi GetDestinationPath video: %v", err)
	}

	expectedVideoSubdir := filepath.Join(tempDir, "2026-10-07", "videos")
	if !strings.HasPrefix(videoPath, expectedVideoSubdir) {
		t.Errorf("Kỳ vọng đường dẫn video bắt đầu bằng '%s', nhận được '%s'", expectedVideoSubdir, videoPath)
	}

	// Xác nhận 2 thư mục con được tạo thực tế trên ổ đĩa
	if fi, err := os.Stat(expectedImageSubdir); err != nil || !fi.IsDir() {
		t.Errorf("Thư mục images chưa được tạo: %v", err)
	}
	if fi, err := os.Stat(expectedVideoSubdir); err != nil || !fi.IsDir() {
		t.Errorf("Thư mục videos chưa được tạo: %v", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test:file?.mp4", "test_file_.mp4"},
		{"<hello>*world|.png", "_hello__world_.png"},
		{"", "file"},
		{"normal_name.mp4", "normal_name.mp4"},
	}

	for _, tt := range tests {
		got := SanitizeFilename(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeFilename(%q) = %q, kỳ vọng %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractInviteHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://t.me/+AbCd12345", "AbCd12345"},
		{"https://t.me/joinchat/XyZ987", "XyZ987"},
		{"+MyCustomHash", "MyCustomHash"},
		{"https://t.me/group_name", "group_name"},
	}

	for _, tt := range tests {
		got := ExtractInviteHash(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractInviteHash(%q) = %q, kỳ vọng %q", tt.input, got, tt.expected)
		}
	}
}
