package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type Service struct {
	cfg        *Config
	state      *DownloadState
	organizer  *MediaOrganizer
	downloader *MediaDownloader
	rejoiner   *RejoinManager

	client     *telegram.Client
	api        *tg.Client
	dispatcher tg.UpdateDispatcher

	targetInputPeer tg.InputPeerClass
	targetChannelID int64
	targetAccess    int64
	targetTitle     string

	mu        sync.Mutex
	isKicked  bool
	rejoinCtx context.CancelFunc
}

func NewService(cfg *Config, state *DownloadState) *Service {
	organizer := NewMediaOrganizer(cfg.Settings.DownloadDir)

	s := &Service{
		cfg:       cfg,
		state:     state,
		organizer: organizer,
	}

	s.dispatcher = tg.NewUpdateDispatcher()
	s.registerUpdateHandlers()

	sessionStorage := &session.FileStorage{
		Path: cfg.Telegram.SessionFile,
	}

	opts := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  s.dispatcher,
		DialTimeout:    15 * time.Second,
	}

	if envOpts, err := telegram.OptionsFromEnvironment(opts); err == nil {
		opts = envOpts
	}

	s.client = telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, opts)
	s.api = s.client.API()

	s.downloader = NewMediaDownloader(s.api, organizer)
	s.rejoiner = NewRejoinManager(
		s.api,
		cfg.Target.Identifier,
		cfg.Target.InviteLink,
		time.Duration(cfg.Settings.RejoinRetryIntervalSec)*time.Second,
	)

	return s
}

// Start khởi động service Telegram và lắng nghe
func (s *Service) Start(ctx context.Context) error {
	Logf("🚀 [Telegram] Đang kết nối tới máy chủ Telegram MTProto...\n")
	return s.client.Run(ctx, func(ctx context.Context) error {
		Logf("✅ [Telegram] Đã kết nối tới MTProto.\n")

		// 1. Xác thực tài khoản
		status, err := s.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("không thể kiểm tra trạng thái xác thực: %w", err)
		}

		if !status.Authorized {
			Logf("🔑 [Telegram] Chưa đăng nhập. Bắt đầu luồng xác thực terminal...\n")
			flow := auth.NewFlow(
				NewTerminalAuth(s.cfg.Telegram.Phone),
				auth.SendCodeOptions{},
			)
			if err := s.client.Auth().IfNecessary(ctx, flow); err != nil {
				return fmt.Errorf("đăng nhập thất bại: %w", err)
			}
			Logf("✅ [Telegram] Đăng nhập thành công và đã lưu phiên làm việc (Session)!\n")
		} else {
			Logf("✅ [Telegram] Đã phục hồi phiên làm việc từ file session.\n")
		}

		// 2. Định danh Target Peer
		if err := s.resolveTarget(ctx); err != nil {
			if s.cfg.Settings.AutoRejoin {
				Logf("⚠️ [Telegram] Chưa vào được nhóm (%v). Bắt đầu auto-join...\n", err)
				if err := s.rejoiner.TryRejoin(ctx); err != nil {
					Logf("❌ [Telegram] Lỗi auto-join lần đầu: %v. Sẽ tiếp tục thử lại...\n", err)
				} else {
					Logf("🎉 [Telegram] Đã join vào nhóm thành công!\n")
					_ = s.resolveTarget(ctx)
				}
			} else {
				Logf("ℹ️ [Telegram] Chế độ xem khách (AutoRejoin: false). Không thực hiện join nhóm: %v\n", err)
			}
		}

		// 3. Quét bù lịch sử nếu được bật
		if s.cfg.Settings.SyncHistory && s.targetInputPeer != nil {
			go func() {
				time.Sleep(2 * time.Second)
				s.syncHistory(ctx)
			}()
		}

		// 4. Bật vòng lặp Polling định kỳ để luôn tự động quét & tải media mới 24/7
		go s.startPollingLoop(ctx)

		Logf("📡 [Telegram] Đang hoạt động và lắng nghe media thời gian thực...\n")
		Logf("📁 [Telegram] Thư mục lưu trữ: %s/<YYYY-MM-DD>/[images|videos]/\n", s.cfg.Settings.DownloadDir)

		<-ctx.Done()
		return ctx.Err()
	})
}

func (s *Service) resolveTarget(ctx context.Context) error {
	idStr := strings.TrimSpace(s.cfg.Target.Identifier)
	if idStr == "" && s.cfg.Target.InviteLink != "" && s.cfg.Settings.AutoRejoin {
		// Thử import invite trước nếu AutoRejoin được bật
		_ = s.rejoiner.TryRejoin(ctx)
	}

	if idStr == "" {
		return fmt.Errorf("chưa cấu hình identifier (username hoặc ID) của nhóm")
	}

	// 1. Kiểm tra trực tiếp trong danh sách các nhóm đã tham gia (Dialogs)
	dialogsRes, err := s.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      100,
	})
	if err == nil {
		var chatList []tg.ChatClass
		switch res := dialogsRes.(type) {
		case *tg.MessagesDialogs:
			chatList = res.Chats
		case *tg.MessagesDialogsSlice:
			chatList = res.Chats
		}

		cleanNumStr := strings.TrimPrefix(strings.TrimPrefix(idStr, "-100"), "-")
		numID, _ := strconv.ParseInt(cleanNumStr, 10, 64)

		for _, rawChat := range chatList {
			switch ch := rawChat.(type) {
			case *tg.Channel:
				if (numID != 0 && ch.ID == numID) || strings.EqualFold(ch.Title, idStr) {
					s.targetChannelID = ch.ID
					s.targetAccess = ch.AccessHash
					s.targetTitle = ch.Title
					s.targetInputPeer = &tg.InputPeerChannel{
						ChannelID:  ch.ID,
						AccessHash: ch.AccessHash,
					}
					s.state.UpdateTarget(ch.ID, ch.Title)
					Logf("🎯 [Telegram] Đã tìm thấy nhóm đã tham gia: '%s' (ID: -100%d)\n", ch.Title, ch.ID)
					return nil
				}
			case *tg.Chat:
				if (numID != 0 && ch.ID == numID) || strings.EqualFold(ch.Title, idStr) {
					s.targetChannelID = ch.ID
					s.targetTitle = ch.Title
					s.targetInputPeer = &tg.InputPeerChat{
						ChatID: ch.ID,
					}
					s.state.UpdateTarget(ch.ID, ch.Title)
					Logf("🎯 [Telegram] Đã tìm thấy nhóm đã tham gia: '%s' (ID: -%d)\n", ch.Title, ch.ID)
					return nil
				}
			}
		}
	}

	// Nếu là link mời (chứa '+' hoặc 'joinchat')
	if strings.Contains(idStr, "+") || strings.Contains(idStr, "joinchat") {
		hash := ExtractInviteHash(idStr)
		checkRes, err := s.api.MessagesCheckChatInvite(ctx, hash)
		if err == nil {
			switch res := checkRes.(type) {
			case *tg.ChatInviteAlready:
				switch ch := res.Chat.(type) {
				case *tg.Channel:
					s.targetChannelID = ch.ID
					s.targetAccess = ch.AccessHash
					s.targetTitle = ch.Title
					s.targetInputPeer = &tg.InputPeerChannel{
						ChannelID:  ch.ID,
						AccessHash: ch.AccessHash,
					}
					s.state.UpdateTarget(ch.ID, ch.Title)
					Logf("🎯 [Telegram] Đã định vị nhóm mục tiêu từ Link mời: '%s' (ID: %d)\n", ch.Title, ch.ID)
					return nil
				case *tg.Chat:
					s.targetChannelID = ch.ID
					s.targetTitle = ch.Title
					s.targetInputPeer = &tg.InputPeerChat{
						ChatID: ch.ID,
					}
					s.state.UpdateTarget(ch.ID, ch.Title)
					Logf("🎯 [Telegram] Đã định vị nhóm mục tiêu từ Link mời: '%s' (ID: %d)\n", ch.Title, ch.ID)
					return nil
				}
			case *tg.ChatInvite:
				Logf("ℹ️ [Telegram] Phát hiện nhóm '%s' qua link mời. Đang tiến hành tham gia...\n", res.Title)
				_, importErr := s.api.MessagesImportChatInvite(ctx, hash)
				if importErr != nil && !tgerr.Is(importErr, "USER_ALREADY_PARTICIPANT") {
					return fmt.Errorf("không thể tham gia nhóm từ link mời: %w", importErr)
				}

				check2, err2 := s.api.MessagesCheckChatInvite(ctx, hash)
				if err2 == nil {
					if already, ok := check2.(*tg.ChatInviteAlready); ok {
						if ch, ok := already.Chat.(*tg.Channel); ok {
							s.targetChannelID = ch.ID
							s.targetAccess = ch.AccessHash
							s.targetTitle = ch.Title
							s.targetInputPeer = &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
							s.state.UpdateTarget(ch.ID, ch.Title)
							Logf("🎯 [Telegram] Đã tham gia và định vị nhóm: '%s' (ID: %d)\n", ch.Title, ch.ID)
							return nil
						}
					}
				}
			}
		} else {
			return fmt.Errorf("kiểm tra link mời '%s' thất bại: %w", idStr, err)
		}
	}

	clean := CleanUsername(idStr)
	resolved, err := s.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: clean})
	if err != nil {
		return fmt.Errorf("không thể resolve '%s': %w", idStr, err)
	}

	for _, chat := range resolved.Chats {
		if ch, ok := chat.(*tg.Channel); ok {
			s.targetChannelID = ch.ID
			s.targetAccess = ch.AccessHash
			s.targetTitle = ch.Title
			s.targetInputPeer = &tg.InputPeerChannel{
				ChannelID:  ch.ID,
				AccessHash: ch.AccessHash,
			}
			s.state.UpdateTarget(ch.ID, ch.Title)
			Logf("🎯 [Telegram] Đã định vị nhóm mục tiêu: '%s' (ID: %d)\n", ch.Title, ch.ID)
			return nil
		}
	}

	return fmt.Errorf("không tìm thấy kênh/nhóm từ %s", idStr)
}

func (s *Service) registerUpdateHandlers() {
	s.dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}

		if peerCh, ok := msg.PeerID.(*tg.PeerChannel); ok {
			if s.targetChannelID != 0 && peerCh.ChannelID != s.targetChannelID {
				return nil
			}
		}

		s.handleMessage(ctx, msg)
		return nil
	})

	s.dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}

		if peerChat, ok := msg.PeerID.(*tg.PeerChat); ok {
			if s.targetChannelID != 0 && peerChat.ChatID != s.targetChannelID {
				return nil
			}
		} else if peerCh, ok := msg.PeerID.(*tg.PeerChannel); ok {
			if s.targetChannelID != 0 && peerCh.ChannelID != s.targetChannelID {
				return nil
			}
		}

		s.handleMessage(ctx, msg)
		return nil
	})

	s.dispatcher.OnChannel(func(ctx context.Context, e tg.Entities, u *tg.UpdateChannel) error {
		// Lắng nghe cập nhật kênh (nếu bị kick)
		return nil
	})
}

func (s *Service) handleMessage(ctx context.Context, msg *tg.Message) {
	if msg == nil || msg.Media == nil {
		return
	}

	if s.state.IsDownloaded(msg.ID) {
		return
	}

	mType, filename, isMedia := s.downloader.ExtractMediaInfo(msg)
	if !isMedia {
		return
	}

	date := time.Unix(int64(msg.Date), 0)
	dateStr := date.Format("2006-01-02")
	Logf("📥 [Telegram] Phát hiện %s mới: %s (ID: %d) -> Thư mục: %s/%s/\n", mType, filename, msg.ID, dateStr, mType)

	filePath, err := s.downloader.DownloadMessageMedia(ctx, msg, nil)
	if err != nil {
		Logf("❌ [Telegram] Tải thất bại ID %d: %v\n", msg.ID, err)
		if tgerr.Is(err, "CHANNEL_PRIVATE") || tgerr.Is(err, "CHAT_ADMIN_REQUIRED") {
			s.triggerKickRecovery(ctx)
		}
		return
	}

	s.state.MarkDownloaded(msg.ID, filePath)
	Logf("✅ [Telegram] Đã lưu thành công: %s\n", filePath)
}

func (s *Service) triggerKickRecovery(ctx context.Context) {
	if !s.cfg.Settings.AutoRejoin {
		Logf("ℹ️ [Telegram] Tài khoản bị kick nhưng AutoRejoin đang tắt (Chế độ xem khách). Bỏ qua vào lại nhóm.\n")
		return
	}

	s.mu.Lock()
	if s.isKicked {
		s.mu.Unlock()
		return
	}
	s.isKicked = true
	s.mu.Unlock()

	Logf("⚠️ [Telegram] Cảnh báo: Tài khoản đã bị Kick hoặc mất quyền truy cập nhóm!\n")
	fmt.Println("🔄 [Telegram] Kích hoạt tiến trình Auto-Rejoin...")

	go s.rejoiner.RejoinLoop(ctx, func() {
		s.mu.Lock()
		s.isKicked = false
		s.mu.Unlock()

		_ = s.resolveTarget(ctx)
		// Quét bù tin nhắn sau khi vào lại nhóm thành công
		go s.syncHistory(ctx)
	})
}

func (s *Service) syncHistory(ctx context.Context) {
	if s.targetInputPeer == nil {
		return
	}

	limitDesc := "toàn bộ (không giới hạn)"
	if s.cfg.Settings.HistoryLimit > 0 {
		limitDesc = fmt.Sprintf("tối đa %d tin nhắn", s.cfg.Settings.HistoryLimit)
	}
	Logf("🔄 [Telegram Sync] Bắt đầu quét lịch sử tin nhắn (%s)...\n", limitDesc)

	offsetID := 0
	totalScanned := 0
	totalDownloaded := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batchLimit := 100
		if s.cfg.Settings.HistoryLimit > 0 {
			remaining := s.cfg.Settings.HistoryLimit - totalScanned
			if remaining <= 0 {
				break
			}
			if remaining < batchLimit {
				batchLimit = remaining
			}
		}

		history, err := s.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:       s.targetInputPeer,
			OffsetID:   offsetID,
			OffsetDate: 0,
			AddOffset:  0,
			Limit:      batchLimit,
			MaxID:      0,
			MinID:      0,
		})
		if err != nil {
			if waitSec, ok := tgerr.AsFloodWait(err); ok {
				Logf("⏳ [Telegram Sync] FloodWait: Cần chờ %d giây trước khi tiếp tục...\n", waitSec)
				time.Sleep(time.Duration(waitSec+2) * time.Second)
				continue
			}
			Logf("⚠️ [Telegram Sync] Lỗi lấy lịch sử: %v\n", err)
			break
		}

		var messages []tg.MessageClass
		switch h := history.(type) {
		case *tg.MessagesMessages:
			messages = h.Messages
		case *tg.MessagesMessagesSlice:
			messages = h.Messages
		case *tg.MessagesChannelMessages:
			messages = h.Messages
		}

		if len(messages) == 0 {
			// Đã quét đến tin nhắn đầu tiên của nhóm
			break
		}

		oldestID := 0
		for _, rawMsg := range messages {
			totalScanned++
			if msg, ok := rawMsg.(*tg.Message); ok {
				if oldestID == 0 || msg.ID < oldestID {
					oldestID = msg.ID
				}
				if !s.state.IsDownloaded(msg.ID) && msg.Media != nil {
					s.handleMessage(ctx, msg)
					totalDownloaded++
				}
			}
		}

		Logf("📊 [Telegram Sync] Tiến trình: đã duyệt %d tin nhắn (tải về %d media mới)... (offset ID: %d)\n",
			totalScanned, totalDownloaded, oldestID)

		if oldestID <= 1 || oldestID == offsetID {
			break
		}
		offsetID = oldestID

		// Tạm dừng 300ms giữa các batch để giữ kết nối ổn định
		time.Sleep(300 * time.Millisecond)
	}

	Logf("✅ [Telegram Sync] Quét lịch sử hoàn tất! Đã duyệt %d tin nhắn, tải về %d media.\n",
		totalScanned, totalDownloaded)
}

// startPollingLoop quét định kỳ để bắt kịp mọi tin nhắn media mới 24/7
func (s *Service) startPollingLoop(ctx context.Context) {
	interval := s.cfg.Settings.PollIntervalSec
	if interval <= 0 {
		interval = 15
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	Logf("🔄 [Telegram Poller] Đã kích hoạt quét tin mới định kỳ (mỗi %d giây)...\n", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			peer := s.targetInputPeer
			kicked := s.isKicked
			s.mu.Unlock()

			if peer == nil || kicked {
				continue
			}
			s.pollNewMessages(ctx)
		}
	}
}

// pollNewMessages lấy các tin nhắn mới nhất và tải ngay nếu có media
func (s *Service) pollNewMessages(ctx context.Context) {
	s.mu.Lock()
	peer := s.targetInputPeer
	s.mu.Unlock()
	if peer == nil {
		return
	}

	history, err := s.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:       peer,
		OffsetID:   0,
		OffsetDate: 0,
		AddOffset:  0,
		Limit:      50,
		MaxID:      0,
		MinID:      0,
	})
	if err != nil {
		if waitSec, ok := tgerr.AsFloodWait(err); ok {
			Logf("⏳ [Telegram Poller] FloodWait: Cần chờ %d giây...\n", waitSec)
			time.Sleep(time.Duration(waitSec+1) * time.Second)
			return
		}
		if tgerr.Is(err, "CHANNEL_PRIVATE") || tgerr.Is(err, "CHAT_ADMIN_REQUIRED") {
			s.triggerKickRecovery(ctx)
		}
		return
	}

	var messages []tg.MessageClass
	switch h := history.(type) {
	case *tg.MessagesMessages:
		messages = h.Messages
	case *tg.MessagesMessagesSlice:
		messages = h.Messages
	case *tg.MessagesChannelMessages:
		messages = h.Messages
	}

	// Duyệt từ cũ đến mới để tải tuần tự
	for i := len(messages) - 1; i >= 0; i-- {
		if msg, ok := messages[i].(*tg.Message); ok {
			if !s.state.IsDownloaded(msg.ID) && msg.Media != nil {
				s.handleMessage(ctx, msg)
			}
		}
	}
}

// ListDialogs liệt kê tất cả các nhóm / kênh mà tài khoản đã tham gia
func (s *Service) ListDialogs(ctx context.Context) error {
	fmt.Println("🚀 [Telegram] Đang kết nối tới máy chủ Telegram MTProto...")
	return s.client.Run(ctx, func(ctx context.Context) error {
		fmt.Println("✅ [Telegram] Đã kết nối tới MTProto.")
		status, err := s.client.Auth().Status(ctx)
		if err != nil || !status.Authorized {
			return fmt.Errorf("tài khoản chưa được xác thực, vui lòng chạy lệnh bình thường để đăng nhập trước")
		}

		dialogsRes, err := s.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      100,
		})
		if err != nil {
			return fmt.Errorf("không thể lấy danh sách nhóm: %w", err)
		}

		var chatList []tg.ChatClass
		switch res := dialogsRes.(type) {
		case *tg.MessagesDialogs:
			chatList = res.Chats
		case *tg.MessagesDialogsSlice:
			chatList = res.Chats
		}

		fmt.Println("================================================================================")
		fmt.Printf("%-20s | %-12s | %s\n", "ID NHÓM (DÙNG ĐỂ TẢI)", "LOẠI", "TÊN NHÓM / KÊNH ĐÃ THAM GIA")
		fmt.Println("--------------------------------------------------------------------------------")
		for _, rawChat := range chatList {
			switch ch := rawChat.(type) {
			case *tg.Channel:
				fmt.Printf("-100%-16d | %-12s | %s\n", ch.ID, "Supergroup", ch.Title)
			case *tg.Chat:
				fmt.Printf("-%-19d | %-12s | %s\n", ch.ID, "Group", ch.Title)
			}
		}
		fmt.Println("================================================================================")
		return nil
	})
}
