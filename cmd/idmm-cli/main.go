package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"idmm/internal/engine"
	"idmm/internal/media"
	"idmm/internal/telegram"
)

func formatBytes(bytes int64) string {
	if bytes < 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	val := float64(bytes)
	unitIdx := 0
	for val >= 1024 && unitIdx < len(units)-1 {
		val /= 1024
		unitIdx++
	}
	return fmt.Sprintf("%.2f %s", val, units[unitIdx])
}

func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return "--"
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %dm", seconds/3600, (seconds%3600)/60)
}

func renderSegmentBar(segments []*engine.Segment, width int) string {
	if len(segments) == 0 {
		return ""
	}
	if width <= 0 {
		width = 30
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, seg := range segments {
		segWidth := width / len(segments)
		if segWidth < 1 {
			segWidth = 1
		}

		total := seg.End - seg.Start + 1
		var pct float64 = 0
		if total > 0 {
			pct = float64(seg.Downloaded) / float64(total)
			if pct > 1 {
				pct = 1
			}
		}

		filled := int(math.Round(pct * float64(segWidth)))
		for f := 0; f < filled; f++ {
			sb.WriteString("=")
		}
		for e := filled; e < segWidth; e++ {
			sb.WriteString(" ")
		}

		if i < len(segments)-1 {
			sb.WriteString("|")
		}
	}
	sb.WriteString("]")
	return sb.String()
}

func main() {
	urlFlag := flag.String("url", "", "URL to download")
	outFlag := flag.String("o", "", "Destination file path")
	connsFlag := flag.Int("c", 8, "Concurrent connection count (default 8)")
	limitFlag := flag.Int64("limit", 0, "Speed limit in KB/s (0 = unlimited)")
	streamFlag := flag.Bool("stream", false, "Enable local HTTP streaming proxy (stream-as-you-download)")
	streamPort := flag.Int("port", 0, "Port for local streaming proxy (0 = auto)")
	probeOnly := flag.Bool("probe", false, "Probe file information and exit")

	// Telegram Downloader Flags
	tgFlag := flag.Bool("telegram", false, "Start Telegram Media Auto-Downloader")
	tgShort := flag.Bool("tg", false, "Alias for -telegram")
	tgConfig := flag.String("tg-config", "config.telegram.yaml", "Telegram config YAML file")
	tgTarget := flag.String("tg-target", "", "Target Telegram group identifier or username")
	tgInvite := flag.String("tg-invite", "", "Telegram invite link for auto re-join")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if *tgFlag || *tgShort {
		runTelegramDownload(ctx, cancel, sigChan, *tgConfig, *tgTarget, *tgInvite, *outFlag)
		return
	}

	targetURL := *urlFlag
	if targetURL == "" && flag.NArg() > 0 {
		targetURL = flag.Arg(0)
	}

	if targetURL == "" {
		fmt.Println("🚀 IDMM (Next-Gen Download Accelerator & Media Streamer CLI)")
		fmt.Println("Usage: idmm-cli -url <URL> [-o output] [-c conns] [-stream] [-limit KB/s]")
		fmt.Println("       idmm-cli -tg [-tg-target <group>] [-tg-invite <link>]")
		fmt.Println("Example: idmm-cli https://example.com/movie.mp4 -stream")
		fmt.Println("Example HLS: idmm-cli https://example.com/stream/index.m3u8 -o video.mp4")
		fmt.Println("Example Telegram: idmm-cli -tg -tg-target @mygroup")
		os.Exit(1)
	}

	// Check if target is an HLS / m3u8 stream
	if strings.Contains(targetURL, ".m3u8") {
		runHLSDownload(ctx, sigChan, targetURL, *outFlag, *connsFlag)
		return
	}

	// Standard Multi-threaded File Download
	runStandardDownload(ctx, sigChan, targetURL, *outFlag, *connsFlag, *limitFlag, *streamFlag, *streamPort, *probeOnly)
}

func runHLSDownload(ctx context.Context, sigChan chan os.Signal, targetURL, outPath string, conns int) {
	if outPath == "" {
		outPath = "video.ts"
	}

	fmt.Println("🎬 [HLS Detected] Initializing concurrent m3u8 playlist streamer...")
	downloader := media.NewHLSDownloader(targetURL, outPath, conns)

	go func() {
		<-sigChan
		fmt.Println("\n\n⏸️  Interrupt received. Canceling HLS download...")
		os.Exit(1)
	}()

	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		for prog := range downloader.ProgressChannel() {
			if prog.Status == "DOWNLOADING" {
				fmt.Printf("\r⬇️  [HLS %5.1f%%] %d / %d Segments | %s/s",
					prog.ProgressPercent,
					prog.DownloadedSegments,
					prog.TotalSegments,
					formatBytes(prog.SpeedBytesPerSec),
				)
			}
		}
	}()

	startTime := time.Now()
	err := downloader.Start(ctx)
	<-doneChan

	if err != nil {
		fmt.Printf("\n❌ HLS download failed: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)
	fmt.Printf("\n\n✅ HLS stream assembled successfully in %s\n", duration.Round(time.Millisecond))
	fmt.Printf("📁 Saved to: %s\n", outPath)
}

func runStandardDownload(ctx context.Context, sigChan chan os.Signal, targetURL, outPath string, conns int, limit int64, enableStream bool, streamPort int, probeOnly bool) {
	cfg := engine.DefaultConfig(targetURL, outPath)
	cfg.Connections = conns
	if limit > 0 {
		cfg.MaxSpeedBytesPerSec = limit * 1024
	}

	downloader := engine.NewDownloader(cfg)

	go func() {
		<-sigChan
		fmt.Println("\n\n⏸️  Interrupt received. Pausing download and saving state...")
		downloader.Pause()
	}()

	fmt.Println("🔍 Probing remote resource...")
	info, err := downloader.Probe(ctx)
	if err != nil {
		fmt.Printf("❌ Probe failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("------------------------------------------------------------")
	fmt.Printf("📄 Filename:      %s\n", info.Filename)
	if info.TotalSize > 0 {
		fmt.Printf("📦 File Size:     %s (%d bytes)\n", formatBytes(info.TotalSize), info.TotalSize)
	} else {
		fmt.Printf("📦 File Size:     Unknown (Streaming / Chunked)\n")
	}
	if info.AcceptRanges {
		fmt.Printf("⚡ Multi-Thread:  Supported (Accept-Ranges: bytes) - Using %d connections\n", cfg.Connections)
	} else {
		fmt.Printf("⚠️ Multi-Thread:  Not supported by server (Single-stream fallback)\n")
	}
	if info.ETag != "" {
		fmt.Printf("🏷️ ETag:           %s\n", info.ETag)
	}
	fmt.Println("------------------------------------------------------------")

	if probeOnly {
		return
	}

	// Initialize Stream-as-you-Download Server if requested
	var streamServer *media.StreamServer
	if enableStream {
		streamServer = media.NewStreamServer(streamPort)
		_, err := streamServer.Start()
		if err != nil {
			fmt.Printf("⚠️ Failed to start streaming proxy: %v\n", err)
		} else {
			taskID := "1"
			streamServer.RegisterTask(taskID, downloader)
			streamURL := streamServer.GetStreamURL(taskID)
			fmt.Println("🎬 [Stream-as-you-Download] Local HTTP Proxy Activated!")
			fmt.Printf("📺 Stream URL:    %s\n", streamURL)
			fmt.Println("👉 Tip: Open VLC or your browser and paste the Stream URL to watch now!")
			fmt.Println("------------------------------------------------------------")
			defer streamServer.Stop()
		}
	}

	startTime := time.Now()

	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		for update := range downloader.ProgressChannel() {
			if update.Status == engine.StatusDownloading {
				segVisual := renderSegmentBar(update.Segments, 24)
				fmt.Printf("\r⬇️  [%5.1f%%] %s / %s | %s/s | ETA: %-6s | Conns: %d %s",
					update.ProgressPercent,
					formatBytes(update.DownloadedBytes),
					formatBytes(update.TotalSize),
					formatBytes(update.SpeedBytesPerSec),
					formatDuration(update.ETASeconds),
					update.ActiveConnections,
					segVisual,
				)
			}
			if update.Status == engine.StatusCompleted || update.Status == engine.StatusFailed || update.Status == engine.StatusPaused {
				return
			}
		}
	}()

	fmt.Printf("🚀 Starting download to: %s\n\n", downloader.TargetPath())
	downloadErr := downloader.Start(ctx)
	<-doneChan

	duration := time.Since(startTime)

	if downloadErr != nil {
		fmt.Printf("\n❌ Download failed: %v\n", downloadErr)
		os.Exit(1)
	}

	if downloader.Status() == engine.StatusPaused {
		fmt.Println("\n💾 Download paused. Resume anytime by re-running the same command!")
		return
	}

	if downloader.Status() == engine.StatusCompleted {
		avgSpeed := int64(0)
		if duration.Seconds() > 0 && info.TotalSize > 0 {
			avgSpeed = int64(float64(info.TotalSize) / duration.Seconds())
		}
		fmt.Printf("\n\n✅ Download completed in %s (Average speed: %s/s)\n",
			duration.Round(time.Millisecond),
			formatBytes(avgSpeed),
		)
		fmt.Printf("📁 Saved to: %s\n", downloader.TargetPath())
	}
}

func runTelegramDownload(ctx context.Context, cancel context.CancelFunc, sigChan chan os.Signal, configPath, targetFlag, inviteFlag, dirFlag string) {
	fmt.Println("================================================================")
	fmt.Println("🚀 IDMM - Telegram Media Auto-Downloader & Streamer")
	fmt.Println("   - Phân loại thư mục: <YYYY-MM-DD>/images & <YYYY-MM-DD>/videos")
	fmt.Println("   - Tự động vào lại nhóm khi bị kick & Tiếp tục tải (Resume)")
	fmt.Println("================================================================")

	cfg, err := telegram.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("❌ Lỗi cấu hình: %v\n", err)
		os.Exit(1)
	}

	if targetFlag != "" {
		cfg.Target.Identifier = targetFlag
	}
	if inviteFlag != "" {
		cfg.Target.InviteLink = inviteFlag
	}
	if dirFlag != "" {
		cfg.Settings.DownloadDir = dirFlag
	}

	stateFile := "state.telegram.json"
	st, err := telegram.LoadState(stateFile)
	if err != nil {
		fmt.Printf("❌ Lỗi khởi tạo state: %v\n", err)
		os.Exit(1)
	}

	svc := telegram.NewService(cfg, st)

	go func() {
		<-sigChan
		fmt.Println("\n🛑 Nhận tín hiệu ngắt. Đang dừng an toàn và lưu trạng thái...")
		cancel()
	}()

	if err := svc.Start(ctx); err != nil && err != context.Canceled {
		fmt.Printf("❌ Lỗi vận hành: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("👋 Đã dừng tiến trình Telegram Downloader thành công.")
}
