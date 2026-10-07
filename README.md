# 🚀 IDMM (Internet Download Manager Modern)

> **Next-Gen Multi-Threaded Download Accelerator & Media Streamer built in Go.**

IDMM is a high-performance open-source alternative to legacy download managers like IDM. It eliminates slow post-download file merging through zero-merge direct-to-disk allocations, features real-time HTTP media streaming (*Stream-as-you-download*), a modern dark mode UI, and seamless browser integration with Google Chrome, Microsoft Edge, and Mozilla Firefox.

This branch (`telegram-intergration`) also includes a dedicated **Telegram Media Downloader & Streamer** module powered by the native Telegram MTProto v2.0 protocol.

---

## ⚡ Key Highlights

* 🚀 **Direct-to-Disk / Zero-Merge Engine:** Pre-allocates target file capacity and parallelizes chunk streams directly via `io.WriterAt`. Completely eliminates slow, resource-heavy `.tmp` concatenation phases.
* 🎬 **Stream-as-you-Download:** Built-in Local HTTP Range Streaming Server (`/stream/`). Prioritizes metadata headers and active playback windows, allowing instant video playback in VLC or embedded web players at just 1–2% progress.
* 📺 **Multi-Threaded HLS / m3u8 Video Downloader:** Automatically sniffs stream playlists, decrypts AES-128 segments concurrently, and packages pristine `.mp4` / `.ts` containers.
* 🖥️ **Modern Desktop Web Dashboard:** Sleek Dark Mode UI with Glassmorphism, real-time throughput sparklines, and an interactive connection segment visualizer.
* ⏰ **Automated Download Scheduler:** Real-time cron scheduling and task queue management directly from the dashboard.
* 🌐 **Browser Extensions (Manifest V3):** Automatically catches downloads and injects floating video sniffer buttons into supported web players across Chrome, Edge, and Firefox.
* 📱 **Telegram Media Downloader (Branch `telegram-intergration`):** Real-time MTProto media listener, automatic date-based sorting into separate `images/` and `videos/` subdirectories, historical message pagination backfilling, and auto-rejoin mechanisms. *(See full documentation at [README.telegram.md](README.telegram.md))*.

---

## 📦 Directory Structure

```text
idmm/
├── dist/IDMM/            # Standalone Distribution Package for Windows
│   ├── idmm.exe          # Main Desktop Application (GUI)
│   ├── idmm-cli.exe      # Command-Line Interface (CLI Tool)
│   ├── idmm-host.exe     # Browser Native Messaging Host Bridge
│   ├── web/              # Web Dashboard UI Assets
│   ├── extension/        # Chrome & Edge Extension (Manifest V3)
│   ├── install.bat       # 1-Click Installer & Registry Registration
│   └── uninstall.bat     # Clean Uninstaller Script
├── bin/                  # Compiled binary executables
├── cmd/                  # Application entry points (idmm, idmm-cli, idmm-host, idmm-telegram)
├── internal/
│   ├── engine/           # Multi-threaded download engine & direct-to-disk chunking
│   ├── media/            # Local HTTP Streaming proxy & HLS downloader
│   ├── scheduler/        # Task scheduling and timer execution
│   ├── server/           # REST APIs, SSE events & WebSocket endpoints
│   ├── nativemsg/        # Cross-browser Native Messaging protocol bridge
│   └── telegram/         # MTProto userbot client, date organizer & history backfiller
├── web/                  # Desktop Web UI (HTML, CSS, JS)
├── extension/            # Chrome & Edge Extension source
└── README.telegram.md    # Dedicated documentation for Telegram integration
```

---

## 🚀 Installation & Usage

### 1. One-Click Setup (Recommended)
Navigate to the `dist\IDMM` folder and double-click:
```cmd
install.bat
```
This automatically registers the Native Messaging Host in Windows Registry for Chrome and Edge.

### 2. Launch Desktop App
Double-click `dist\IDMM\idmm.exe` (or `bin\idmm.exe`).  
The standalone modern Dark Mode dashboard will launch, allowing you to add URLs, schedule jobs, configure bandwidth limits, adjust connections, and stream media in real time.

### 3. Command-Line Interface (CLI)
Download HTTP/HTTPS files with 16 connections:
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://proof.ovh.net/files/10Mb.dat" -c 16
```

Download and stream concurrently (*Stream-as-you-download*):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/movie.mp4" -stream
```

Download HLS stream video (.m3u8):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/playlist.m3u8" -o "video.mp4" -c 16
```

Download Telegram media automatically (Auto Re-join & Date-based sorting):
```powershell
# Run with a specific target group and backup invite link
.\dist\IDMM\idmm-cli.exe -tg -tg-target "@group_username" -tg-invite "https://t.me/+inviteHash"
```
> Media files are automatically organized under: `downloads/<YYYY-MM-DD>/images/` and `downloads/<YYYY-MM-DD>/videos/`.

### 4. Browser Extension Installation (Chrome / Edge)
1. Open your browser and navigate to the extension manager:
   * **Chrome:** `chrome://extensions`
   * **Edge:** `edge://extensions`
2. Enable **Developer mode** (toggle in the top-right corner).
3. Click **"Load unpacked"** and select the folder: `dist\IDMM\extension` (or `D:\Personal\idmm\extension`).
4. When downloading files or streaming web video, IDMM automatically intercepts requests and maximizes transfer speeds!

---

## 🛠️ Testing & Verification

### Run Unit Tests:
```powershell
go test -v ./...
```
All packages including `internal/engine`, `internal/media`, `internal/scheduler`, `internal/server`, `internal/nativemsg`, and `internal/telegram` achieve **100% PASS**.

### Run End-to-End Build Suite:
```powershell
pwsh -File .\scripts\build_dist.ps1
```

---

## 📄 License
MIT License. Open source and built for speed.
