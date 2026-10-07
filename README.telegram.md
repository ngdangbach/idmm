# 🚀 IDMM Telegram Media Downloader & Streamer
> **Branch:** `telegram-intergration` | **Language:** Go (Golang 1.25+) | **Protocol:** Telegram MTProto v2.0 (`gotd/td`)

A dedicated high-performance Telegram Userbot Client extension for **IDMM**. It automatically downloads, categorizes, and synchronizes media (videos and photos) from Telegram groups and channels directly to your local drive in real time, while also supporting deep historical message pagination backfilling.

---

## 🌟 Key Features

* 📁 **Automatic Date-Based Organization & Media Splitting:**
  - Files are automatically categorized into structured directories:
    ```text
    D:/tmp/ (or downloads/)
    └── 2026-10-07/
        ├── images/         # All photos & images (.jpg, .png)
        │   ├── photo_1001_20261007_123000.jpg
        │   └── ...
        └── videos/         # All video files (.mp4, .mkv)
            ├── 1002_sample_video.mp4
            └── ...
    ```
* 📜 **Full History Pagination Backfill:**
  - Traverses past chat history to download thousands of historical media files.
  - Configurable history limit (`history_limit: 1000`, or set to `0` to download entire chat history from day one).
* ⚡ **Real-Time Event Listener:**
  - Listens to incoming MTProto channel/group updates and streams media to disk immediately as soon as a new post is published.
* 🛡️ **Supports Both Public and Private Groups/Channels:**
  - Flexible target resolution: Public usernames (`@group_name`), private invite links (`https://t.me/+...`), or numeric chat IDs (`-100...`).
* 👻 **Anonymous Guest Preview Mode (`auto_rejoin: false`):**
  - For public channels/groups, fetches media via public preview without joining the group—completely invisible to group admins.
* 🔄 **Auto Re-join on Kick (`auto_rejoin: true`):**
  - For private groups, automatically re-joins using backup invite links if the account gets kicked, equipped with exponential backoff to handle `FLOOD_WAIT` safely.
* 🔑 **Single-Time Interactive Login & Token Persistence:**
  - Automatically saves the authorized MTProto session token to `session.telegram.json`. Subsequent runs authenticate silently in the background without prompting for phone numbers or 2FA passwords.
* 💾 **Deduplication & Resume Tracking:**
  - Tracks all downloaded message IDs and filenames in `state.telegram.json`. Stop and resume the process at any time without duplicate downloads.

---

## 📦 Directory Structure

```text
idmm/
├── bin/
│   ├── idmm-telegram.exe         # Dedicated standalone Telegram downloader binary
│   ├── idmm-cli.exe              # IDMM CLI with integrated -tg flags
│   ├── config.telegram.yaml      # Telegram runtime configuration
│   └── session.telegram.json     # Saved MTProto authentication session
├── cmd/
│   ├── idmm-telegram/            # Standalone idmm-telegram entry point
│   │   └── main.go
│   └── idmm-cli/                 # Main IDMM CLI supporting -telegram / -tg
│       └── main.go
├── internal/
│   └── telegram/
│       ├── config.go             # YAML parser & Smart Config Resolver
│       ├── auth.go               # Interactive CLI OTP & 2FA authentication flow
│       ├── organizer.go          # YYYY-MM-DD/images & videos path organizer
│       ├── organizer_test.go     # Unit tests for organizer & invite hash parser
│       ├── state.go              # state.telegram.json persistence & deduplication
│       ├── state_test.go         # Unit tests for state persistence
│       ├── rejoiner.go           # Auto Re-join handler & invite link resolver
│       ├── downloader.go         # MTProto streaming chunk downloader
│       └── service.go            # Connection lifecycle, update dispatcher & pagination
└── config.telegram.yaml          # Root sample configuration template
```

---

## ⚙️ Configuration Guide (`config.telegram.yaml`)

Sample configuration file located at `config.telegram.yaml` (or `bin/config.telegram.yaml`):

```yaml
telegram:
  api_id: 12345678
  api_hash: "0123456789abcdef0123456789abcdef"
  session_file: "session.telegram.json"
  phone: "" # Leave empty to prompt interactively in terminal on first run

target:
  # Public username (@group), private invite link (https://t.me/+...), or numeric ID
  identifier: "https://t.me/+AbCdEfGhIjK..."
  # Backup invite link for auto-rejoining if kicked (for private groups)
  invite_link: "https://t.me/+AbCdEfGhIjK..."

settings:
  # Destination directory on disk (e.g., "D:/tmp" or "./downloads")
  download_dir: "./downloads"
  # Auto-rejoin if kicked (false = guest preview for public groups; true = auto-rejoin for private groups)
  auto_rejoin: false
  # Retry interval in seconds before attempting to re-join
  rejoin_retry_interval_sec: 60
  # Enable backfilling past chat history
  sync_history: true
  # Number of past messages to scan (1000 = recent 1000 messages; 0 = entire channel history)
  history_limit: 1000
  # Maximum concurrent chunk downloads
  max_concurrent_downloads: 3
```

---

## 🚀 Usage Guide

### Method 1: Standalone `idmm-telegram.exe` (Recommended)
```powershell
# Run using the default config file
.\bin\idmm-telegram.exe

# Override target and destination directory via CLI flags
.\bin\idmm-telegram.exe -target "https://t.me/+AbCdEfGhIjK..." -dir "D:/tmp"

# Specify custom configuration file path
.\bin\idmm-telegram.exe -config "C:/path/to/my_config.yaml"
```

### Method 2: Integrated IDMM CLI (`idmm-cli.exe`)
```powershell
# Enable Telegram Downloader mode
.\bin\idmm-cli.exe -tg

# Override target group and invite link
.\bin\idmm-cli.exe -tg -tg-target "@my_group" -tg-invite "https://t.me/+inviteHash"
```

---

## 🛠️ Automated Unit Tests

Run the full Telegram test suite:
```powershell
go test -v ./internal/telegram
```

Expected result:
```text
=== RUN   TestMediaOrganizer_Subfolders
--- PASS: TestMediaOrganizer_Subfolders (0.00s)
=== RUN   TestSanitizeFilename
--- PASS: TestSanitizeFilename (0.00s)
=== RUN   TestExtractInviteHash
--- PASS: TestExtractInviteHash (0.00s)
=== RUN   TestDownloadState_Persistence
--- PASS: TestDownloadState_Persistence (0.00s)
PASS
ok      idmm/internal/telegram  1.439s
```

---

## 📌 Important Notes

1. **First-Time Interactive Login:**
   - On the initial run, the terminal prompts for your phone number in international format (e.g., `+84987654321`) and sends an OTP code to your Telegram client.
   - Enter the OTP code (and your 2FA password if enabled).
   - Once verified, `session.telegram.json` is generated to persist authentication permanently.
2. **Smart Config Resolution:**
   - Whether executed from repository root `idmm/` or binary folder `bin/`, the application automatically locates and resolves configuration and session files located either in the current directory or adjacent to the executable.
