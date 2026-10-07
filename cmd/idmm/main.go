package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"idmm/internal/server"
)

func findWebDir() string {
	// 1. Current working directory ./web
	if _, err := os.Stat("web"); err == nil {
		return "web"
	}

	// 2. Relative to executable
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, "web")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		// In case binary is in ./bin
		candidateParent := filepath.Join(exeDir, "..", "web")
		if _, err := os.Stat(candidateParent); err == nil {
			return candidateParent
		}
	}

	return "web"
}

func main() {
	portFlag := flag.Int("port", 8989, "Server port (default: 8989)")
	noWindow := flag.Bool("no-window", false, "Do not launch native app window")
	webDirFlag := flag.String("web", "", "Path to web directory (optional)")
	flag.Parse()

	webDir := *webDirFlag
	if webDir == "" {
		webDir = findWebDir()
	}

	srv := server.NewServer(webDir, *portFlag)
	baseURL, err := srv.Start()
	if err != nil {
		fmt.Printf("❌ Failed to start IDMM server: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=====================================================================")
	fmt.Println("🚀 IDMM Desktop Application (Next-Gen Download Accelerator)")
	fmt.Println("⚡ Engine: Direct-to-Disk Sparse Write (Zero-Merge Time) & Range Proxy")
	fmt.Printf("🌐 Dashboard: %s\n", baseURL)
	fmt.Println("=====================================================================")

	if !*noWindow {
		fmt.Println("🖥️  Launching native desktop application window...")
		_ = srv.LaunchAppMode(baseURL)
	}

	// Listen for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 IDMM shutting down gracefully...")
}
