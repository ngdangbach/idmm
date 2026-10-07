package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type DownloadState struct {
	TargetChatID       int64          `json:"target_chat_id"`
	TargetTitle        string         `json:"target_title"`
	LastProcessedMsgID int            `json:"last_processed_msg_id"`
	DownloadedFiles    map[int]string `json:"downloaded_files"` // msgID -> local file path
	TotalDownloaded    int            `json:"total_downloaded"`

	mu       sync.RWMutex `json:"-"`
	filePath string       `json:"-"`
}

func LoadState(filePath string) (*DownloadState, error) {
	st := &DownloadState{
		DownloadedFiles: make(map[int]string),
		filePath:        filePath,
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, fmt.Errorf("không thể đọc state: %w", err)
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("không thể parse state JSON: %w", err)
	}

	if st.DownloadedFiles == nil {
		st.DownloadedFiles = make(map[int]string)
	}
	st.TotalDownloaded = len(st.DownloadedFiles)
	st.filePath = filePath
	return st, nil
}

func (s *DownloadState) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("lỗi encode state: %w", err)
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func (s *DownloadState) IsDownloaded(msgID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.DownloadedFiles[msgID]
	return exists
}

func (s *DownloadState) MarkDownloaded(msgID int, path string) error {
	s.mu.Lock()
	s.DownloadedFiles[msgID] = path
	s.TotalDownloaded = len(s.DownloadedFiles)
	if msgID > s.LastProcessedMsgID {
		s.LastProcessedMsgID = msgID
	}
	s.mu.Unlock()

	return s.Save()
}

func (s *DownloadState) UpdateTarget(chatID int64, title string) error {
	s.mu.Lock()
	s.TargetChatID = chatID
	s.TargetTitle = title
	s.mu.Unlock()

	return s.Save()
}

func (s *DownloadState) SetLastProcessedID(msgID int) error {
	s.mu.Lock()
	if msgID > s.LastProcessedMsgID {
		s.LastProcessedMsgID = msgID
	}
	s.mu.Unlock()

	return s.Save()
}
