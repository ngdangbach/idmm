package telegram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadState_Persistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "idmm_state_test_*")
	if err != nil {
		t.Fatalf("Lỗi tạo thư mục tạm: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stateFile := filepath.Join(tempDir, "state.test.json")

	// 1. Khởi tạo state mới
	st, err := LoadState(stateFile)
	if err != nil {
		t.Fatalf("Lỗi LoadState: %v", err)
	}

	if st.LastProcessedMsgID != 0 {
		t.Errorf("LastProcessedMsgID ban đầu phải bằng 0, nhận được %d", st.LastProcessedMsgID)
	}

	// 2. Cập nhật target và đánh dấu đã tải
	if err := st.UpdateTarget(123456789, "Test Channel"); err != nil {
		t.Fatalf("Lỗi UpdateTarget: %v", err)
	}

	if err := st.MarkDownloaded(1001, "downloads/2026-10-07/videos/video_1001.mp4"); err != nil {
		t.Fatalf("Lỗi MarkDownloaded: %v", err)
	}

	if !st.IsDownloaded(1001) {
		t.Errorf("Kỳ vọng message 1001 được đánh dấu là IsDownloaded")
	}

	if st.IsDownloaded(1002) {
		t.Errorf("Kỳ vọng message 1002 chưa được đánh dấu")
	}

	if st.LastProcessedMsgID != 1001 {
		t.Errorf("LastProcessedMsgID phải cập nhật lên 1001, nhận %d", st.LastProcessedMsgID)
	}

	// 3. Load lại từ đĩa xem có khớp dữ liệu không
	reloaded, err := LoadState(stateFile)
	if err != nil {
		t.Fatalf("Lỗi reload state từ đĩa: %v", err)
	}

	if reloaded.TargetChatID != 123456789 {
		t.Errorf("TargetChatID reload sai: %d", reloaded.TargetChatID)
	}

	if reloaded.TargetTitle != "Test Channel" {
		t.Errorf("TargetTitle reload sai: %s", reloaded.TargetTitle)
	}

	if !reloaded.IsDownloaded(1001) {
		t.Errorf("Message 1001 phải tồn tại sau khi reload")
	}
}
