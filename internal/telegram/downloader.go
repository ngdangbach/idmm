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

// ExtractMediaInfo phân tích tin nhắn để xác định loại media, tên file và thông tin tải
func (m *MediaDownloader) ExtractMediaInfo(msg *tg.Message) (mType MediaType, filename string, isMedia bool) {
	if msg == nil || msg.Media == nil {
		return "", "", false
	}

	date := time.Unix(int64(msg.Date), 0)
	datePrefix := date.Format("20060102_150405")

	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		// Ảnh từ Telegram
		filename = fmt.Sprintf("photo_%d_%s.jpg", msg.ID, datePrefix)
		return MediaTypeImage, filename, true

	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return "", "", false
		}

		// Xác định tên file gốc nếu có trong DocumentAttributeFilename
		origName := ""
		isVideo := false
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *tg.DocumentAttributeFilename:
				origName = a.FileName
			case *tg.DocumentAttributeVideo:
				isVideo = true
			}
		}

		mime := strings.ToLower(doc.MimeType)
		if strings.HasPrefix(mime, "video/") || isVideo {
			mType = MediaTypeVideo
			ext := ".mp4"
			if origName != "" {
				ext = filepath.Ext(origName)
			}
			if ext == "" {
				ext = ".mp4"
			}
			if origName != "" {
				filename = fmt.Sprintf("%d_%s", msg.ID, origName)
			} else {
				filename = fmt.Sprintf("video_%d_%s%s", msg.ID, datePrefix, ext)
			}
			return mType, filename, true
		} else if strings.HasPrefix(mime, "image/") {
			mType = MediaTypeImage
			ext := ".jpg"
			if origName != "" {
				ext = filepath.Ext(origName)
			}
			if origName != "" {
				filename = fmt.Sprintf("%d_%s", msg.ID, origName)
			} else {
				filename = fmt.Sprintf("image_%d_%s%s", msg.ID, datePrefix, ext)
			}
			return mType, filename, true
		}

		// Tài liệu khác (nếu cần có thể lưu hoặc bỏ qua)
		return MediaTypeOther, fmt.Sprintf("doc_%d_%s", msg.ID, origName), false
	}

	return "", "", false
}

// DownloadMessageMedia thực hiện tải media và lưu vào đúng thư mục images/ hoặc videos/
func (m *MediaDownloader) DownloadMessageMedia(ctx context.Context, msg *tg.Message, progress ProgressCallback) (string, error) {
	mType, filename, isMedia := m.ExtractMediaInfo(msg)
	if !isMedia {
		return "", fmt.Errorf("tin nhắn không chứa ảnh hoặc video hợp lệ")
	}

	msgDate := time.Unix(int64(msg.Date), 0)
	destPath, err := m.organizer.GetDestinationPath(msgDate, mType, filename)
	if err != nil {
		return "", err
	}

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
		_, err = builder.ToPath(ctx, destPath)
		if err != nil {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("lỗi tải ảnh: %w", err)
		}

	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return "", fmt.Errorf("không thể đọc dữ liệu document")
		}

		location := doc.AsInputDocumentFileLocation("")
		builder := m.dl.Download(m.api, location)
		_, err = builder.ToPath(ctx, destPath)
		if err != nil {
			_ = os.Remove(destPath)
			return "", fmt.Errorf("lỗi tải video/document: %w", err)
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
