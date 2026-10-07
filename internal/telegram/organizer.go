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

// GetDestinationPath tạo đường dẫn đích theo cấu trúc:
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

	// Nếu file đã tồn tại và trùng tên nhưng khác size, thêm hậu tố tránh ghi đè
	if _, err := os.Stat(finalPath); err == nil {
		ext := filepath.Ext(cleanName)
		base := strings.TrimSuffix(cleanName, ext)
		finalPath = filepath.Join(targetDir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano()%10000, ext))
	}

	return finalPath, nil
}
