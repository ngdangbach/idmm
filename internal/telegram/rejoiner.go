package telegram

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// RejoinManager quản lý logic tự động tham gia lại nhóm khi bị kick/rời nhóm
type RejoinManager struct {
	api        *tg.Client
	inviteLink string
	identifier string
	retryDelay time.Duration
}

func NewRejoinManager(api *tg.Client, identifier, inviteLink string, retryDelay time.Duration) *RejoinManager {
	if retryDelay <= 0 {
		retryDelay = 30 * time.Second
	}
	return &RejoinManager{
		api:        api,
		identifier: identifier,
		inviteLink: inviteLink,
		retryDelay: retryDelay,
	}
}

// ExtractInviteHash trích xuất hash từ các định dạng link invite
func ExtractInviteHash(rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if strings.HasPrefix(rawLink, "+") {
		return strings.TrimPrefix(rawLink, "+")
	}

	u, err := url.Parse(rawLink)
	if err == nil && u.Path != "" {
		p := strings.Trim(u.Path, "/")
		if strings.HasPrefix(p, "+") {
			return strings.TrimPrefix(p, "+")
		}
		if strings.HasPrefix(p, "joinchat/") {
			return strings.TrimPrefix(p, "joinchat/")
		}
		// Có thể hash nằm ngay sau domain: t.me/hash
		parts := strings.Split(p, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return rawLink
}

// CleanUsername làm sạch username Telegram
func CleanUsername(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "@")
	if strings.Contains(raw, "t.me/") {
		parts := strings.Split(raw, "t.me/")
		if len(parts) > 1 {
			raw = strings.Trim(parts[1], "/")
		}
	}
	return raw
}

// JoinChannel tham gia kênh/nhóm public qua username
func (r *RejoinManager) JoinChannel(ctx context.Context, username string) error {
	clean := CleanUsername(username)
	resolved, err := r.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: clean})
	if err != nil {
		return fmt.Errorf("không thể tìm thấy username @%s: %w", clean, err)
	}

	for _, chat := range resolved.Chats {
		if ch, ok := chat.(*tg.Channel); ok {
			inputChannel := &tg.InputChannel{
				ChannelID:  ch.ID,
				AccessHash: ch.AccessHash,
			}
			_, err := r.api.ChannelsJoinChannel(ctx, inputChannel)
			if err != nil {
				if tgerr.Is(err, "USER_ALREADY_PARTICIPANT") {
					return nil
				}
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("không tìm thấy channel/group từ username @%s", clean)
}

// JoinByInviteLink tham gia nhóm qua link mời
func (r *RejoinManager) JoinByInviteLink(ctx context.Context, inviteLink string) error {
	hash := ExtractInviteHash(inviteLink)
	if hash == "" {
		return fmt.Errorf("link invite không hợp lệ: %s", inviteLink)
	}

	_, err := r.api.MessagesImportChatInvite(ctx, hash)
	if err != nil {
		if tgerr.Is(err, "USER_ALREADY_PARTICIPANT") {
			return nil
		}
		return err
	}
	return nil
}

// TryRejoin thử tham gia lại nhóm, ưu tiên invite link rồi tới username
func (r *RejoinManager) TryRejoin(ctx context.Context) error {
	var lastErr error

	if r.inviteLink != "" {
		err := r.JoinByInviteLink(ctx, r.inviteLink)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	if r.identifier != "" && !strings.HasPrefix(r.identifier, "-") {
		err := r.JoinChannel(ctx, r.identifier)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("không có link mời hoặc username để tham gia lại")
}

// RejoinLoop tự động thử lại khi bị kick, có xử lý FloodWait và retry backoff
func (r *RejoinManager) RejoinLoop(ctx context.Context, onRejoined func()) {
	delay := r.retryDelay
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		Logf("🔄 [Auto-Rejoin] Đang thử kết nối lại vào nhóm...\n")
		err := r.TryRejoin(ctx)
		if err == nil {
			Logf("🎉 [Auto-Rejoin] Đã tham gia lại nhóm thành công!\n")
			if onRejoined != nil {
				onRejoined()
			}
			return
		}

		// Kiểm tra nếu Telegram bắt chờ FloodWait
		if waitSec, ok := tgerr.AsFloodWait(err); ok {
			Logf("⏳ [Auto-Rejoin] Telegram FloodWait: Cần chờ %d giây trước khi thử lại...\n", waitSec)
			time.Sleep(time.Duration(waitSec+2) * time.Second)
			continue
		}

		if tgerr.Is(err, "USER_BANNED_IN_CHANNEL") {
			Logf("⚠️ [Auto-Rejoin] Tài khoản đang bị Admin cấm (Banned). Sẽ thử lại sau %v...\n", delay)
		} else {
			Logf("❌ [Auto-Rejoin] Lỗi tham gia nhóm (%v). Sẽ thử lại sau %v...\n", err, delay)
		}

		time.Sleep(delay)
		if delay < 10*time.Minute {
			delay = delay * 3 / 2 // Tăng dần thời gian chờ
		}
	}
}
