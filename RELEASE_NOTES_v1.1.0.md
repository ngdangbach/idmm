# IDMM v1.1.0 - BitTorrent Engine & Multi-Browser Native Messaging

**Release Version:** `v1.1.0`  
**Release Date:** 2026-10-07  
**Artifact Package:** [`IDMM-v1.1.0-windows-amd64.zip`](file:///d:/Personal/idmm/IDMM-v1.1.0-windows-amd64.zip)

---

## 🌟 Highlights & Major Features

### 1. 🧲 Native BitTorrent & Magnet Link Engine
IDMM now natively supports peer-to-peer downloading alongside traditional HTTP/HTTPS multi-connection acceleration and HLS video streaming:
- **Magnet URI Support:** Download directly via `magnet:?xt=urn:btih:...` with automatic DHT / tracker discovery and piece verification.
- **Local `.torrent` File Support:** Parse and download `.torrent` files with multi-file selection support.
- **Dedicated Upload Endpoint:** Added `/api/tasks/upload-torrent` multipart API for instant `.torrent` ingestion.
- **Peer-to-Peer Protocol Compliance:** Built with DHT, PEX, tracker announces, and dynamic port allocation to avoid local port conflicts.

### 2. 🎨 Enhanced Modern Web Dashboard UI
The Web UI has been updated to make Torrent downloads effortless:
- **Top Header Action:** Added `🧲 Add Torrent` quick-action button in the top navigation bar to select `.torrent` files directly from your PC.
- **Sidebar Category Filter:** Added `🧲 Torrents & Magnets` filter in the left sidebar to isolate and monitor torrent transfers.
- **Improved "Add Download" Modal:**
  - Expanded URL field accepting HTTP/HTTPS, M3U8, and `magnet:?xt=...` without browser validation errors.
  - Quick **📁 .torrent** file picker button next to the URL input.
  - Informative helper tooltip for user guidance.

### 3. 💻 CLI Torrent Tooling (`idmm-cli.exe`)
- Added command: `idmm-cli torrent <magnet-link-or-file.torrent> [-out <directory>]`
- Live terminal progress meter showing active peers, download speed (MB/s), and completed pieces.

### 4. 🦊 Full Mozilla Firefox Support
- **Signed XPI Package:** Included `idmm-firefox.xpi` compatible with Firefox toolbar and popup controls.
- **Universal Installer (`install.bat`):** 1-Click native messaging host registration across **Google Chrome**, **Microsoft Edge**, and **Mozilla Firefox**.

---

## 📦 Package Contents (`IDMM-v1.1.0-windows-amd64.zip`)

| File / Folder | Description |
| :--- | :--- |
| `idmm.exe` | Main desktop application & local HTTP/WebSocket server |
| `idmm-cli.exe` | Standalone CLI client supporting HTTP, HLS, and BitTorrent |
| `idmm-host.exe` | Native Messaging host for browser extensions |
| `web/` | Web dashboard assets (`index.html`, `app.js`, `style.css`) |
| `extension/` | Chrome & Edge extension bundle (Manifest V3) |
| `extension-firefox/` | Firefox extension bundle |
| `idmm-firefox.xpi` | Ready-to-install Firefox Add-on |
| `install.bat` | 1-Click setup: Registers Native Messaging & creates Desktop shortcut |
| `uninstall.bat` | 1-Click cleanup tool |

---

## 🚀 How to Upgrade / Install

1. Download and extract **`IDMM-v1.1.0-windows-amd64.zip`**.
2. Run **`install.bat`** as Administrator to register browser extensions and desktop shortcut.
3. Launch **`idmm.exe`** to open the IDMM Web Dashboard (`http://localhost:5244`).
