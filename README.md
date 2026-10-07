# 🚀 IDMM (Internet Download Manager Modern)

> **Next-Gen Multi-Threaded Download Accelerator, BitTorrent Engine & Media Streamer built in Go.**

IDMM is a high-performance open-source alternative to legacy download managers like IDM. It eliminates slow post-download file merging through zero-merge direct-to-disk allocations, features real-time HTTP media streaming (*Stream-as-you-download*), native BitTorrent & Magnet link acceleration, modern dark mode UI, and seamless browser integration across Google Chrome, Microsoft Edge, and Mozilla Firefox.

---

## ⚡ Key Highlights

* 🚀 **Direct-to-Disk / Zero-Merge Engine:** Pre-allocates target file capacity and parallelizes chunk streams directly via `io.WriterAt`. Completely eliminates slow, resource-heavy `.tmp` concatenation phases.
* 🧲 **Native BitTorrent & Magnet Support:** Full peer-to-peer engine powered by DHT, Trackers, and Peer Swarm. Automatically resolves and accelerates `.torrent` files and `magnet:?xt=...` links via CLI and Dashboard.
* 🎬 **Stream-as-you-Download:** Built-in Local HTTP Range Streaming Server (`/stream/`). Prioritizes metadata headers and active playback windows, allowing instant video playback in VLC or embedded web players at just 1–2% progress.
* 📺 **Multi-Threaded HLS / m3u8 Video Downloader:** Automatically sniffs stream playlists, decrypts AES-128 segments concurrently, and packages pristine `.mp4` / `.ts` containers.
* 🖥️ **Modern Desktop Web Dashboard:** Sleek Dark Mode UI with Glassmorphism, real-time throughput sparklines, and an interactive connection segment visualizer.
* ⏰ **Automated Download Scheduler:** Real-time cron scheduling and task queue management directly from the dashboard.
* 🌐 **Cross-Browser Extensions (Manifest V3 & Gecko):**
  * **Google Chrome & Microsoft Edge:** Manifest V3 extension with automatic download interception and floating video sniffer button.
  * **Mozilla Firefox:** Gecko WebExtension MV3 with native messaging bridge and toolbar integration.

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
│   ├── extension-firefox/# Mozilla Firefox Extension (Gecko MV3)
│   ├── idmm-firefox.xpi  # Packaged Firefox XPI Add-on
│   ├── install.bat       # 1-Click Installer & Registry Registration
│   └── uninstall.bat     # Clean Uninstaller Script
├── bin/                  # Compiled binary executables
├── cmd/                  # Entry points (idmm, idmm-cli, idmm-host)
├── internal/
│   ├── engine/           # Multi-threaded download engine & direct-to-disk chunking
│   ├── torrent/          # BitTorrent P2P engine & Magnet URI resolver
│   ├── media/            # Local HTTP Streaming proxy & HLS downloader
│   ├── scheduler/        # Task scheduling and timer execution
│   ├── server/           # REST APIs, SSE events & WebSocket endpoints
│   └── nativemsg/        # Cross-browser Native Messaging protocol bridge
├── web/                  # Desktop Web UI (HTML, CSS, JS)
├── extension/            # Chrome/Edge Extension source
├── extension-firefox/    # Firefox Extension source
└── ...
```

---

## 🚀 Installation & Usage

### 1. One-Click Setup (Recommended)
Navigate to the `dist\IDMM` folder and double-click:
```cmd
install.bat
```
This automatically registers the Native Messaging Host in Windows Registry for **Google Chrome, Microsoft Edge, and Mozilla Firefox**, and generates a desktop shortcut.

### 2. Launch the Desktop App
Double-click `dist\IDMM\idmm.exe` (or `bin\idmm.exe`).  
The standalone modern Dark Mode dashboard will launch, allowing you to add URLs, schedule jobs, configure bandwidth limits, adjust connections, and stream media in real time.

### 3. Command-Line Interface (CLI)
Download HTTP/HTTPS files with 16 connections:
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://proof.ovh.net/files/10Mb.dat" -c 16
```

Download via **BitTorrent (.torrent or Magnet link)**:
```powershell
# Download using a local .torrent file
.\dist\IDMM\idmm-cli.exe -torrent "ubuntu-24.04.torrent" -o "D:/Downloads"

# Download using a Magnet link
.\dist\IDMM\idmm-cli.exe -magnet "magnet:?xt=urn:btih:..." -o "D:/Downloads"

# Auto-detects input type directly:
.\dist\IDMM\idmm-cli.exe "magnet:?xt=urn:btih:..."
```

Download and stream concurrently (*Stream-as-you-download*):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/movie.mp4" -stream
```

Download HLS stream video (.m3u8):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/playlist.m3u8" -o "video.mp4" -c 16
```

### 4. Browser Extension Setup

#### A. For Google Chrome & Microsoft Edge:
1. Open your browser and navigate to the extension manager:
   * **Chrome:** `chrome://extensions`
   * **Edge:** `edge://extensions`
2. Enable **Developer mode** (toggle in the top-right corner).
3. Click **"Load unpacked"** and select the folder: `dist\IDMM\extension` (or `D:\Personal\idmm\extension`).

#### B. For Mozilla Firefox:
1. **Load Extension for Development / Testing:**
   * Open Firefox and navigate to: `about:debugging#/runtime/this-firefox`
   * Click **"Load Temporary Add-on..."**.
   * Select `manifest.json` inside `dist\IDMM\extension-firefox` (or `D:\Personal\idmm\extension-firefox`).
2. **Pin Icon to Toolbar:**
   * Click the **Extensions (puzzle piece icon 🧩)** in the top-right toolbar.
   * Locate **IDMM — Next-Gen Download Accelerator** -> click the **Gear ⚙️ icon** (or right-click) -> select **"Pin to Toolbar"**.
3. **Permanent Installation (.xpi):**
   * Pre-packaged bundle available at `dist\IDMM\idmm-firefox.xpi`.
   * On Firefox Dev / Beta / Nightly / ESR: Navigate to `about:config`, set `xpinstall.signatures.required` to `false`, then drag and drop `idmm-firefox.xpi` into Firefox.
   * On standard Firefox: Submit to [addons.mozilla.org](https://addons.mozilla.org/developers/) (Unlisted / Self-distribution mode) for automated signing within 2 minutes.

---

## 🛠️ Testing & Verification

Run the comprehensive unit test suite:
```powershell
go test -v ./...
```
All packages (`internal/engine`, `internal/torrent`, `internal/media`, `internal/scheduler`, `internal/server`, `internal/nativemsg`) achieve **100% PASS**.

Build standalone distribution packages:
```powershell
pwsh -File .\scripts\build_dist.ps1
```

---

## 📄 License
MIT License. Open source and built for speed.
