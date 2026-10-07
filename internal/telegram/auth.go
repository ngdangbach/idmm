package telegram

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

// TerminalAuth triển khai auth.UserAuthenticator để đăng nhập Telegram tương tác từ Terminal
type TerminalAuth struct {
	phone string
}

func NewTerminalAuth(phone string) *TerminalAuth {
	return &TerminalAuth{phone: phone}
}

func (t *TerminalAuth) Phone(ctx context.Context) (string, error) {
	if t.phone != "" {
		return t.phone, nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("📱 Nhập số điện thoại Telegram (ví dụ: +84987654321): ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	t.phone = strings.TrimSpace(input)
	return t.phone, nil
}

func (t *TerminalAuth) Password(ctx context.Context) (string, error) {
	fmt.Print("🔐 Tài khoản có bật 2FA. Nhập mật khẩu 2FA: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePassword)), nil
}

func (t *TerminalAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (t *TerminalAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("📩 Nhập mã xác thực OTP gửi về Telegram: ")
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func (t *TerminalAuth) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("tài khoản chưa được đăng ký trên Telegram")
}
