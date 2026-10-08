package telegram

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type MediaType string

const (
	MediaTypeImage MediaType = "images"
	MediaTypeVideo MediaType = "videos"
	MediaTypeOther MediaType = "other"
)

var invalidChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

// SanitizeFilename làm sạch tên file để tránh lỗi hệ điều hành
func SanitizeFilename(name string) string {
	clean := invalidChars.ReplaceAllString(name, "_")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "file"
	}
	if len(clean) > 180 {
		clean = clean[:180]
	}
	return clean
}

// MediaOrganizer quản lý việc tạo thư mục và đường dẫn theo ngày và loại media
type MediaOrganizer struct {
	baseDir string
}

func NewMediaOrganizer(baseDir string) *MediaOrganizer {
	return &MediaOrganizer{
		baseDir: baseDir,
	}
}

// GetDestinationPath tạo đường dẫn đích theo cấu trúc chuẩn xác:
// <baseDir>/<YYYY-MM-DD>/<images|videos>/<filename>
func (o *MediaOrganizer) GetDestinationPath(date time.Time, mType MediaType, filename string) (string, error) {
	dateStr := date.Format("2006-01-02")
	typeFolder := string(mType)
	if typeFolder == "" {
		typeFolder = string(MediaTypeOther)
	}

	targetDir := filepath.Join(o.baseDir, dateStr, typeFolder)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("không thể tạo thư mục lưu trữ '%s': %w", targetDir, err)
	}

	cleanName := SanitizeFilename(filename)
	finalPath := filepath.Join(targetDir, cleanName)
	return finalPath, nil
}

// FileExistsWithMatchingSize kiểm tra xem file đã tồn tại trên đĩa và có kích thước hợp lệ hay không.
// Nếu expectedSize > 0: file phải có kích thước đúng bằng expectedSize.
// Nếu expectedSize <= 0: chỉ cần file tồn tại và dung lượng > 0.
func (o *MediaOrganizer) FileExistsWithMatchingSize(filePath string, expectedSize int64) bool {
	fi, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return false
	}
	if expectedSize > 0 {
		return fi.Size() == expectedSize
	}
	return fi.Size() > 0
}

// FindExistingMediaFile tìm kiếm file media đã tồn tại trên đĩa:
// Ưu tiên 1: Tên file đầy đủ theo chuẩn (ví dụ: 1234_video.mp4)
// Ưu tiên 2: Tên file gốc (ví dụ: video.mp4) nếu khớp chính xác kích thước file
func (o *MediaOrganizer) FindExistingMediaFile(date time.Time, mType MediaType, filename, origName string, expectedSize int64) (string, bool) {
	dateStr := date.Format("2006-01-02")
	typeFolder := string(mType)
	if typeFolder == "" {
		typeFolder = string(MediaTypeOther)
	}
	targetDir := filepath.Join(o.baseDir, dateStr, typeFolder)

	// Kiểm tra tên file chuẩn (ví dụ 1234_video.mp4)
	stdPath := filepath.Join(targetDir, SanitizeFilename(filename))
	if o.FileExistsWithMatchingSize(stdPath, expectedSize) {
		return stdPath, true
	}

	// Kiểm tra tên file gốc (ví dụ video.mp4) nếu khác stdPath
	if origName != "" {
		origPath := filepath.Join(targetDir, SanitizeFilename(origName))
		if origPath != stdPath && o.FileExistsWithMatchingSize(origPath, expectedSize) {
			return origPath, true
		}
	}

	return "", false
}

// FormatFileSize định dạng dung lượng byte thành chuỗi dễ đọc (B, KB, MB, GB)
func FormatFileSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
