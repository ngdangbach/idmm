package telegram

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

type ProgressCallback func(downloaded, total int64)

type MediaDownloader struct {
	api       *tg.Client
	dl        *downloader.Downloader
	organizer *MediaOrganizer
}

func NewMediaDownloader(api *tg.Client, organizer *MediaOrganizer) *MediaDownloader {
	return &MediaDownloader{
		api:       api,
		dl:        downloader.NewDownloader(),
		organizer: organizer,
	}
}

// MediaInfo chứa thông tin chi tiết về file media phân tích từ tin nhắn Telegram
type MediaInfo struct {
	Type         MediaType
	Filename     string
	OrigName     string
	ExpectedSize int64
	IsMedia      bool
}

// ExtractMediaInfo phân tích tin nhắn để xác định loại media, tên file, tên gốc và dung lượng dự kiến
func (m *MediaDownloader) ExtractMediaInfo(msg *tg.Message) MediaInfo {
	if msg == nil || msg.Media == nil {
		return MediaInfo{}
	}

	date := time.Unix(int64(msg.Date), 0)
	datePrefix := date.Format("20060102_150405")

	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		var expectedSize int64
		if ok {
			for _, s := range photo.Sizes {
				switch sz := s.(type) {
				case *tg.PhotoSize:
					if int64(sz.Size) > expectedSize {
						expectedSize = int64(sz.Size)
					}
				case *tg.PhotoSizeProgressive:
					if len(sz.Sizes) > 0 {
						last := int64(sz.Sizes[len(sz.Sizes)-1])
						if last > expectedSize {
							expectedSize = last
						}
					}
				}
			}
		}
		// Ảnh từ Telegram
		filename := fmt.Sprintf("photo_%d_%s.jpg", msg.ID, datePrefix)
		return MediaInfo{
			Type:         MediaTypeImage,
			Filename:     filename,
			OrigName:     "",
			ExpectedSize: expectedSize,
			IsMedia:      true,
		}

	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return MediaInfo{}
		}

		expectedSize := doc.Size

		// Xác định tên file gốc nếu có trong DocumentAttributeFilename
		origName := ""
		isVideo := false
		isSticker := false
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeFilename:
				origName = a.FileName
			case *tg.DocumentAttributeVideo:
				isVideo = true
			case *tg.DocumentAttributeSticker, *tg.DocumentAttributeCustomEmoji:
				isSticker = true
			}
		}

		// Nếu là sticker hoặc custom emoji của Telegram, bỏ qua không tải
		if isSticker {
			return MediaInfo{
				Type:         MediaTypeOther,
				Filename:     fmt.Sprintf("sticker_%d", msg.ID),
				OrigName:     origName,
				ExpectedSize: expectedSize,
				IsMedia:      false,
			}
		}

		mime := strings.ToLower(doc.MimeType)
		fileExt := strings.ToLower(filepath.Ext(origName))

		// Bỏ qua file .webm (sticker video Telegram)
		if fileExt == ".webm" || mime == "video/webm" {
			return MediaInfo{
				Type:         MediaTypeOther,
				Filename:     fmt.Sprintf("sticker_%d.webm", msg.ID),
				OrigName:     origName,
				ExpectedSize: expectedSize,
				IsMedia:      false,
			}
		}

		if strings.HasPrefix(mime, "video/") || isVideo {
			ext := ".mp4"
			if origName != "" {
				ext = filepath.Ext(origName)
			}
			if ext == "" {
				ext = ".mp4"
			}
			var filename string
			if origName != "" {
				filename = fmt.Sprintf("%d_%s", msg.ID, origName)
			} else {
				filename = fmt.Sprintf("video_%d_%s%s", msg.ID, datePrefix, ext)
			}
			return MediaInfo{
				Type:         MediaTypeVideo,
				Filename:     filename,
				OrigName:     origName,
				ExpectedSize: expectedSize,
				IsMedia:      true,
			}
		} else if strings.HasPrefix(mime, "image/") {
			ext := ".jpg"
			if origName != "" {
				ext = filepath.Ext(origName)
			}
			var filename string
			if origName != "" {
				filename = fmt.Sprintf("%d_%s", msg.ID, origName)
			} else {
				filename = fmt.Sprintf("image_%d_%s%s", msg.ID, datePrefix, ext)
			}
			return MediaInfo{
				Type:         MediaTypeImage,
				Filename:     filename,
				OrigName:     origName,
				ExpectedSize: expectedSize,
				IsMedia:      true,
			}
		}

		// Tài liệu khác (nếu cần có thể lưu hoặc bỏ qua)
		return MediaInfo{
			Type:         MediaTypeOther,
			Filename:     fmt.Sprintf("doc_%d_%s", msg.ID, origName),
			OrigName:     origName,
			ExpectedSize: expectedSize,
			IsMedia:      false,
		}
	}

	return MediaInfo{}
}

// DownloadMessageMedia thực hiện tải media và lưu vào đúng thư mục images/ hoặc videos/
func (m *MediaDownloader) DownloadMessageMedia(ctx context.Context, msg *tg.Message, progress ProgressCallback) (string, error) {
	info := m.ExtractMediaInfo(msg)
	if !info.IsMedia {
		return "", fmt.Errorf("tin nhắn không chứa ảnh hoặc video hợp lệ")
	}

	msgDate := time.Unix(int64(msg.Date), 0)
	destPath, err := m.organizer.GetDestinationPath(msgDate, info.Type, info.Filename)
	if err != nil {
		return "", err
	}

	tmpPath := destPath + ".tmp"

	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return "", fmt.Errorf("không thể đọc dữ liệu photo")
		}

		// Tìm size lớn nhất
		var largestType string
		for _, s := range photo.Sizes {
			switch sz := s.(type) {
			case *tg.PhotoSize:
				largestType = sz.Type
			case *tg.PhotoSizeProgressive:
				largestType = sz.Type
			}
		}

		location := &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     largestType,
		}

		builder := m.dl.Download(m.api, location)
		_, err = builder.ToPath(ctx, tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("lỗi tải ảnh: %w", err)
		}

		_ = os.Remove(destPath)
		if err := os.Rename(tmpPath, destPath); err != nil {
			return "", fmt.Errorf("lỗi lưu ảnh từ file tạm: %w", err)
		}

	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return "", fmt.Errorf("không thể đọc dữ liệu document")
		}

		location := doc.AsInputDocumentFileLocation("")
		builder := m.dl.Download(m.api, location)
		_, err = builder.ToPath(ctx, tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("lỗi tải video/document: %w", err)
		}

		_ = os.Remove(destPath)
		if err := os.Rename(tmpPath, destPath); err != nil {
			return "", fmt.Errorf("lỗi lưu video từ file tạm: %w", err)
		}

	default:
		return "", fmt.Errorf("định dạng media không hỗ trợ")
	}

	return destPath, nil
}

// Helper copy stream nếu cần pipe cho local stream proxy
func PipeStream(src io.Reader, dst io.Writer) (int64, error) {
	return io.Copy(dst, src)
}
