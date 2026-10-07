package telegram

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Target   TargetConfig   `yaml:"target"`
	Settings SettingsConfig `yaml:"settings"`
}

type TelegramConfig struct {
	APIID       int    `yaml:"api_id"`
	APIHash     string `yaml:"api_hash"`
	SessionFile string `yaml:"session_file"`
	Phone       string `yaml:"phone"`
}

type TargetConfig struct {
	Identifier string `yaml:"identifier"`  // Username, ID, or Link
	InviteLink string `yaml:"invite_link"` // Link invite để tự động join lại khi bị kick
}

type SettingsConfig struct {
	DownloadDir            string `yaml:"download_dir"`
	AutoRejoin             bool   `yaml:"auto_rejoin"` // false: Chế độ khách xem/preview (không join); true: Tự động join lại nếu bị kick
	RejoinRetryIntervalSec int    `yaml:"rejoin_retry_interval_sec"`
	SyncHistory            bool   `yaml:"sync_history"`
	HistoryLimit           int    `yaml:"history_limit"` // 0 = toàn bộ lịch sử từ đầu; hoặc giới hạn số lượng tin (ví dụ: 1000)
	MaxConcurrentDownloads int    `yaml:"max_concurrent_downloads"`
}

func DefaultConfig() *Config {
	return &Config{
		Telegram: TelegramConfig{
			APIID:       0,
			APIHash:     "",
			SessionFile: "session.telegram.json",
		},
		Target: TargetConfig{
			Identifier: "",
			InviteLink: "",
		},
		Settings: SettingsConfig{
			DownloadDir:            "./downloads",
			AutoRejoin:             false,
			RejoinRetryIntervalSec: 60,
			SyncHistory:            true,
			HistoryLimit:           0,
			MaxConcurrentDownloads: 3,
		},
	}
}

// ResolveConfigPath tự động tìm kiếm file config:
// 1. Thư mục cùng cấp với file thực thi .exe (được ưu tiên nếu người dùng chỉnh sửa trong bin/)
// 2. Đường dẫn CWD
// 3. Thư mục cha của file thực thi .exe
func ResolveConfigPath(filePath string) string {
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, filePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if _, err := os.Stat(filePath); err == nil {
		absPath, err := filepath.Abs(filePath)
		if err == nil {
			return absPath
		}
		return filePath
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		parentCandidate := filepath.Join(filepath.Dir(exeDir), filePath)
		if _, err := os.Stat(parentCandidate); err == nil {
			return parentCandidate
		}
	}

	return filePath
}

func LoadConfig(filePath string) (*Config, error) {
	resolvedPath := ResolveConfigPath(filePath)
	absConfigPath, _ := filepath.Abs(resolvedPath)
	fmt.Printf("📄 [Config] Đang nạp cấu hình từ: %s\n", absConfigPath)

	cfg := DefaultConfig()

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Lưu file mẫu nếu chưa có
			SaveConfig(resolvedPath, cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("không thể đọc file cấu hình '%s': %w", resolvedPath, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("không thể parse YAML: %w", err)
	}

	if cfg.Telegram.APIID == 0 || cfg.Telegram.APIHash == "" {
		return nil, fmt.Errorf("api_id và api_hash bắt buộc phải có giá trị")
	}

	// Chuẩn hóa SessionFile: ưu tiên tìm file session có sẵn ở cạnh config hoặc exe
	if !filepath.IsAbs(cfg.Telegram.SessionFile) {
		configDir := filepath.Dir(absConfigPath)
		candidate := filepath.Join(configDir, cfg.Telegram.SessionFile)
		if _, err := os.Stat(candidate); err == nil {
			cfg.Telegram.SessionFile = candidate
		} else if exePath, err := os.Executable(); err == nil {
			exeCandidate := filepath.Join(filepath.Dir(exePath), cfg.Telegram.SessionFile)
			if _, err := os.Stat(exeCandidate); err == nil {
				cfg.Telegram.SessionFile = exeCandidate
			} else {
				cfg.Telegram.SessionFile = candidate
			}
		}
	}
	fmt.Printf("🔑 [Config] File phiên đăng nhập (Session): %s\n", cfg.Telegram.SessionFile)

	return cfg, nil
}

func SaveConfig(filePath string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("không thể mã hóa cấu hình: %w", err)
	}
	return os.WriteFile(filePath, data, 0644)
}
