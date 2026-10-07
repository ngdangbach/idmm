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

const (
	HostName           = "com.idmm.downloader"
	FirefoxExtensionID = "idmm-downloader@personal.local"
)

func registerHost() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.Abs(exePath)
	dir := filepath.Dir(exePath)

	// 1. Chrome / Chromium / Edge Native Host Manifest
	chromeManifestPath := filepath.Join(dir, HostName+".json")
	chromeManifest := map[string]interface{}{
		"name":        HostName,
		"description": "IDMM Next-Gen Download Accelerator Native Host",
		"path":        exePath,
		"type":        "stdio",
		"allowed_origins": []string{
			"chrome-extension://*",
		},
	}

	chromeData, err := json.MarshalIndent(chromeManifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(chromeManifestPath, chromeData, 0644); err != nil {
		return fmt.Errorf("failed to write chrome manifest: %w", err)
	}
	fmt.Printf("✅ Created Chrome/Edge manifest: %s\n", chromeManifestPath)

	// 2. Mozilla Firefox Native Host Manifest
	firefoxManifestPath := filepath.Join(dir, HostName+".firefox.json")
	firefoxManifest := map[string]interface{}{
		"name":        HostName,
		"description": "IDMM Next-Gen Download Accelerator Native Host for Firefox",
		"path":        exePath,
		"type":        "stdio",
		"allowed_extensions": []string{
			FirefoxExtensionID,
		},
	}

	firefoxData, err := json.MarshalIndent(firefoxManifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(firefoxManifestPath, firefoxData, 0644); err != nil {
		return fmt.Errorf("failed to write firefox manifest: %w", err)
	}
	fmt.Printf("✅ Created Firefox manifest: %s\n", firefoxManifestPath)

	// 3. Register in Windows Registry
	if runtime.GOOS == "windows" {
		targets := map[string]string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + HostName:    chromeManifestPath,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + HostName: chromeManifestPath,
			`HKCU\Software\Mozilla\NativeMessagingHosts\` + HostName:        firefoxManifestPath,
		}

		for regKey, targetPath := range targets {
			cmd := exec.Command("reg", "add", regKey, "/ve", "/t", "REG_SZ", "/d", targetPath, "/f")
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("⚠️ Registry registration warning (%s): %v, output: %s\n", regKey, err, string(out))
			} else {
				fmt.Printf("✅ Registered in Registry: %s\n", regKey)
			}
		}
	}

	return nil
}

func unregisterHost() error {
	if runtime.GOOS == "windows" {
		targets := []string{
			`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + HostName,
			`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + HostName,
			`HKCU\Software\Mozilla\NativeMessagingHosts\` + HostName,
		}

		for _, regKey := range targets {
			cmd := exec.Command("reg", "delete", regKey, "/f")
			_ = cmd.Run()
			fmt.Printf("🗑️ Removed Registry key: %s\n", regKey)
		}
	}
	return nil
}

func main() {
	regFlag := flag.Bool("register", false, "Register Native Messaging Host with Chrome, Edge, and Firefox")
	unregFlag := flag.Bool("unregister", false, "Unregister Native Messaging Host from Chrome, Edge, and Firefox")
	serverURL := flag.String("server", "http://127.0.0.1:8989", "IDMM backend server URL")
	flag.Parse()

	if *regFlag {
		if err := registerHost(); err != nil {
			fmt.Printf("❌ Registration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("🎉 IDMM Native Messaging Host registered successfully for Chrome, Edge, and Firefox!")
		return
	}

	if *unregFlag {
		if err := unregisterHost(); err != nil {
			fmt.Printf("❌ Unregistration failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("🎉 IDMM Native Messaging Host unregistered successfully!")
		return
	}

	// Normal execution: run stdio loop communicating with browser
	host := nativemsg.NewHost(*serverURL)
	host.RunLoop()
}
