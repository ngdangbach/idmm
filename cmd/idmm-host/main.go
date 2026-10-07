package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"idmm/internal/nativemsg"
)

const HostName = "com.idmm.downloader"

func registerHost() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.Abs(exePath)
	dir := filepath.Dir(exePath)

	manifestPath := filepath.Join(dir, HostName+".json")

	manifest := map[string]interface{}{
		"name":        HostName,
		"description": "IDMM Next-Gen Download Accelerator Native Host",
		"path":        exePath,
		"type":        "stdio",
		"allowed_origins": []string{
			"chrome-extension://*",
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	fmt.Printf("✅ Created manifest: %s\n", manifestPath)

	if runtime.GOOS == "windows" {
		targets := []string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + HostName,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + HostName,
		}

		for _, regKey := range targets {
			cmd := exec.Command("reg", "add", regKey, "/ve", "/t", "REG_SZ", "/d", manifestPath, "/f")
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("⚠️ Registry registration warning (%s): %v, output: %s\n", regKey, err, string(out))
			} else {
				fmt.Printf("✅ Registered in Registry: %s\n", regKey)
			}
		}
	}

	return nil
}

func main() {
	regFlag := flag.Bool("register", false, "Register Native Messaging Host with Chrome and Edge")
	serverURL := flag.String("server", "http://127.0.0.1:8989", "IDMM backend server URL")
	flag.Parse()

	if *regFlag {
		if err := registerHost(); err != nil {
			fmt.Printf("❌ Registration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("🎉 IDMM Native Messaging Host registered successfully!")
		return
	}

	// Normal execution: run stdio loop communicating with browser
	host := nativemsg.NewHost(*serverURL)
	host.RunLoop()
}
