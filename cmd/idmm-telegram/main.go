package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"idmm/internal/telegram"
)

func main() {
	configPath := flag.String("config", "config.telegram.yaml", "Đường dẫn file cấu hình YAML")
	targetFlag := flag.String("target", "", "Username hoặc ID của nhóm Telegram (ghi đè file config)")
	inviteFlag := flag.String("invite", "", "Invite link để tự động join lại nếu bị kick (ghi đè file config)")
	dirFlag := flag.String("dir", "", "Thư mục lưu trữ media (ghi đè file config)")
	listFlag := flag.Bool("list", false, "Liệt kê danh sách tất cả các nhóm / kênh bạn đã tham gia")
	flag.Parse()

	fmt.Println("================================================================")
	fmt.Println("🚀 IDMM - Telegram Media Auto-Downloader & Streamer")
	fmt.Println("   - Phân loại thư mục: <YYYY-MM-DD>/images & <YYYY-MM-DD>/videos")
	fmt.Println("   - Tự động vào lại nhóm khi bị kick & Tiếp tục tải (Resume)")
	fmt.Println("================================================================")

	cfg, err := telegram.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("❌ Lỗi cấu hình: %v\n", err)
		pauseAndExit(1)
	}

	if *targetFlag != "" {
		cfg.Target.Identifier = *targetFlag
	}
	if *inviteFlag != "" {
		cfg.Target.InviteLink = *inviteFlag
	}
	if *dirFlag != "" {
		cfg.Settings.DownloadDir = *dirFlag
	}

	stateFile := "state.telegram.json"
	st, err := telegram.LoadState(stateFile)
	if err != nil {
		fmt.Printf("❌ Lỗi khởi tạo state: %v\n", err)
		pauseAndExit(1)
	}

	svc := telegram.NewService(cfg, st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *listFlag {
		if err := svc.ListDialogs(ctx); err != nil {
			fmt.Printf("❌ Lỗi: %v\n", err)
			pauseAndExit(1)
		}
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n🛑 Nhận tín hiệu dừng. Đang lưu trạng thái và ngắt kết nối...")
		cancel()
	}()

	if err := svc.Start(ctx); err != nil && err != context.Canceled {
		fmt.Printf("❌ Lỗi vận hành Telegram Downloader: %v\n", err)
		pauseAndExit(1)
	}

	fmt.Println("👋 Đã dừng tiến trình an toàn.")
}

func pauseAndExit(code int) {
	fmt.Println("\n👉 Nhấn Enter để đóng cửa sổ...")
	_, _ = os.Stdin.Read(make([]byte, 1))
	os.Exit(code)
}
