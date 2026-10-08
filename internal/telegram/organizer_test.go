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

func TestFindExistingMediaFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "idmm_dedup_test_*")
	if err != nil {
		t.Fatalf("Không thể tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	organizer := NewMediaOrganizer(tempDir)
	testDate := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)

	// Tạo thư mục video
	videoDir := filepath.Join(tempDir, "2026-10-08", "videos")
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		t.Fatalf("Lỗi tạo thư mục: %v", err)
	}

	// Trường hợp 1: File gốc rose.mp4 (size 200 bytes) đã có trên đĩa
	rosePath := filepath.Join(videoDir, "rose.mp4")
	dummyData := make([]byte, 200)
	if err := os.WriteFile(rosePath, dummyData, 0644); err != nil {
		t.Fatalf("Lỗi tạo file mẫu: %v", err)
	}

	// Thử tìm với filename "101_rose.mp4", origName "rose.mp4", expectedSize = 200
	foundPath, found := organizer.FindExistingMediaFile(testDate, MediaTypeVideo, "101_rose.mp4", "rose.mp4", 200)
	if !found {
		t.Fatalf("Kỳ vọng tìm thấy file rose.mp4 đã có sẵn trên đĩa")
	}
	if foundPath != rosePath {
		t.Errorf("Đường dẫn tìm thấy %s không khớp %s", foundPath, rosePath)
	}

	// Trường hợp 2: Sai kích thước (expectedSize = 500 nhưng file chỉ có 200) -> Không coi là file đã hoàn tất
	_, foundMismatch := organizer.FindExistingMediaFile(testDate, MediaTypeVideo, "101_rose.mp4", "rose.mp4", 500)
	if foundMismatch {
		t.Errorf("Kỳ vọng KHÔNG tìm thấy khi kích thước không khớp")
	}

	// Trường hợp 3: File chuẩn 202_clip.mp4 đã có trên đĩa
	clipPath := filepath.Join(videoDir, "202_clip.mp4")
	if err := os.WriteFile(clipPath, dummyData, 0644); err != nil {
		t.Fatalf("Lỗi tạo file mẫu: %v", err)
	}

	foundPath2, found2 := organizer.FindExistingMediaFile(testDate, MediaTypeVideo, "202_clip.mp4", "clip.mp4", 200)
	if !found2 || foundPath2 != clipPath {
		t.Errorf("Kỳ vọng tìm thấy file chuẩn 202_clip.mp4")
	}
}

func TestFormatFileSize(t *testing.T) {
	if got := FormatFileSize(0); got != "0 B" {
		t.Errorf("FormatFileSize(0) = %s, kỳ vọng 0 B", got)
	}
	if got := FormatFileSize(1024); got != "1.0 KB" {
		t.Errorf("FormatFileSize(1024) = %s, kỳ vọng 1.0 KB", got)
	}
	if got := FormatFileSize(1048576); got != "1.0 MB" {
		t.Errorf("FormatFileSize(1048576) = %s, kỳ vọng 1.0 MB", got)
	}
}
