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
	"idmm/internal/torrent"
)

func isTorrentInput(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if strings.HasPrefix(lower, "magnet:?") {
		return true
	}
	if strings.HasSuffix(lower, ".torrent") {
		return true
	}
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(info.Name()), ".torrent") {
			return true
		}
	}
	return false
}

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
	torrentFlag := flag.String("torrent", "", "Path or URL to .torrent file")
	magnetFlag := flag.String("magnet", "", "Magnet URI (magnet:?xt=...)")
	outFlag := flag.String("o", "", "Destination file path or directory")
	connsFlag := flag.Int("c", 8, "Concurrent connection count (default 8)")
	limitFlag := flag.Int64("limit", 0, "Speed limit in KB/s (0 = unlimited)")
	streamFlag := flag.Bool("stream", false, "Enable local HTTP streaming proxy (stream-as-you-download)")
	streamPort := flag.Int("port", 0, "Port for local streaming proxy (0 = auto)")
	probeOnly := flag.Bool("probe", false, "Probe file information and exit")
	flag.Parse()

	targetURL := *urlFlag
	if *torrentFlag != "" {
		targetURL = *torrentFlag
	} else if *magnetFlag != "" {
		targetURL = *magnetFlag
	} else if targetURL == "" && flag.NArg() > 0 {
		targetURL = flag.Arg(0)
	}

	if targetURL == "" {
		fmt.Println("🚀 IDMM (Next-Gen Download Accelerator & Media Streamer CLI)")
		fmt.Println("Usage: idmm-cli -url <URL> [-o output] [-c conns] [-stream] [-limit KB/s]")
		fmt.Println("       idmm-cli -torrent <file.torrent> [-o output]")
		fmt.Println("       idmm-cli -magnet <magnet_uri> [-o output]")
		fmt.Println("Example: idmm-cli https://example.com/movie.mp4 -stream")
		fmt.Println("Example Torrent: idmm-cli ubuntu-24.04.torrent -o ./downloads")
		fmt.Println("Example Magnet:  idmm-cli \"magnet:?xt=urn:btih:...\"")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Check if target is a BitTorrent / Magnet link
	if isTorrentInput(targetURL) {
		runTorrentDownload(ctx, sigChan, targetURL, *outFlag)
		return
	}

	// Check if target is an HLS / m3u8 stream
	if strings.Contains(targetURL, ".m3u8") {
		runHLSDownload(ctx, sigChan, targetURL, *outFlag, *connsFlag)
		return
	}

	// Standard Multi-threaded File Download
	runStandardDownload(ctx, sigChan, targetURL, *outFlag, *connsFlag, *limitFlag, *streamFlag, *streamPort, *probeOnly)
}

func runTorrentDownload(ctx context.Context, sigChan chan os.Signal, targetInput, outDir string) {
	if outDir == "" {
		outDir = "./downloads"
	}
	_ = os.MkdirAll(outDir, 0755)

	fmt.Println("🧲 [BitTorrent Detected] Initializing IDMM P2P Torrent Engine...")
	fmt.Printf("📁 Output directory: %s\n", outDir)

	cfg := torrent.DefaultClientConfig(outDir)
	client, err := torrent.NewClient(cfg)
	if err != nil {
		fmt.Printf("❌ Failed to initialize torrent engine: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("⏳ Resolving torrent metadata from peers & trackers (DHT active)...")
	dl, err := client.AddInput(ctx, targetInput)
	if err != nil {
		fmt.Printf("❌ Failed adding torrent input: %v\n", err)
		os.Exit(1)
	}

	metaCtx, metaCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer metaCancel()

	err = dl.WaitForMetadata(metaCtx)
	if err != nil {
		fmt.Printf("❌ Failed retrieving metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("------------------------------------------------------------")
	fmt.Printf("📄 Torrent Name:   %s\n", dl.Name())
	fmt.Printf("📦 Total Size:     %s (%d bytes)\n", formatBytes(dl.TotalLength()), dl.TotalLength())
	files := dl.Files()
	fmt.Printf("📑 Included Files: %d file(s)\n", len(files))
	for i, f := range files {
		if i < 5 {
			fmt.Printf("   ├─ [%d] %s (%s)\n", i+1, f.Path, formatBytes(f.Length))
		} else if i == 5 {
			fmt.Printf("   └─ ... and %d more file(s)\n", len(files)-5)
			break
		}
	}
	fmt.Println("------------------------------------------------------------")

	dl.Start()
	fmt.Println("🚀 Torrent download started! Connecting to peer swarm...")

	go func() {
		<-sigChan
		fmt.Println("\n\n⏸️  Interrupt received. Gracefully closing P2P connections...")
		dl.Drop()
		_ = client.Close()
		os.Exit(1)
	}()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := dl.Stats()
			fmt.Printf("\r⬇️  [Torrent %5.1f%%] %s / %s | %s/s | Peers: %d | ETA: %s   ",
				stats.Progress,
				formatBytes(stats.CompletedBytes),
				formatBytes(stats.TotalBytes),
				formatBytes(stats.DownloadSpeed),
				stats.ConnectedPeers,
				stats.ETA,
			)

			if stats.Progress >= 100.0 {
				duration := time.Since(startTime)
				fmt.Printf("\n\n🎉 [100.0%%] Torrent download finished successfully in %s!\n", duration.Round(time.Second))
				fmt.Printf("📁 Destination: %s\n", dl.SavePath())
				return
			}
		}
	}
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
