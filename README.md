# 🚀 IDMM (Internet Download Manager Modern)

> **Next-Gen Multi-Threaded Download Accelerator & Media Streamer built in Go.**

IDMM là giải pháp thay thế mã nguồn mở hiện đại cho IDM truyền thống, khắc phục hoàn toàn nhược điểm ghép file chậm chạp sau khi tải, hỗ trợ vừa tải vừa xem video trực tiếp (Stream-as-you-download), giao diện Dark Mode cao cấp và tích hợp sâu với trình duyệt Chrome / Edge.

---

## ⚡ Điểm nổi bật

* 🚀 **Ghi đĩa trực tiếp (Direct-to-Disk / Zero-Merge):** Cấp phát trước kích thước tệp và sử dụng `io.WriterAt` để các luồng ghi song song trực tiếp vào tệp đích. Hoàn toàn không mất thời gian ghép các tệp tạm (`.tmp`).
* 🎬 **Stream-as-you-Download:** Tích hợp Local HTTP Streaming Server (`/stream/`). Ưu tiên kéo các byte đầu (metadata) và cửa sổ xem hiện tại, cho phép mở xem video trong VLC hoặc trình phát tích hợp ngay khi mới tải 1–2%.
* 📺 **Tải video HLS / m3u8 đa luồng:** Tự động bắt luồng stream, giải mã AES-128 và ghép thành file `.mp4`/`.ts` hoàn chỉnh.
* 🖥️ **Desktop App Hiện Đại:** Giao diện Dark Mode với Glassmorphism, biểu đồ thông lượng thời gian thực (Speed Sparkline) và thanh trực quan hóa tiến độ từng luồng kết nối (Segment Visualizer Bar).
* ⏰ **Lập lịch tải tự động (Download Scheduler):** Hẹn giờ bắt đầu tải thông minh theo thời gian thực (hỗ trợ hẹn giờ trực tiếp từ Dashboard).
* 🌐 **Browser Extension (Manifest V3):** Tự động bắt link tải trên Chrome / Edge, tích hợp nút nổi *"Download with IDMM"* trên các trình phát video web.

---

## 📦 Cấu trúc Thư mục

```text
idmm/
├── dist/IDMM/            # Bộ phân phối độc lập Standalone Windows Package
│   ├── idmm.exe          # Ứng dụng Desktop chính (7.6 MB)
│   ├── idmm-cli.exe      # Công cụ dòng lệnh CLI (7.5 MB)
│   ├── idmm-host.exe     # Native Messaging Host (7.0 MB)
│   ├── web/              # Bundle tài nguyên Web UI
│   ├── extension/        # Chrome/Edge Extension Manifest V3
│   ├── install.bat       # Script cài đặt 1-click & đăng ký Registry
│   └── uninstall.bat     # Script gỡ cài đặt sạch sẽ
├── bin/                  # Các file nhị phân biên dịch
├── cmd/                  # Mã nguồn ứng dụng (idmm, idmm-cli, idmm-host)
├── internal/
│   ├── engine/           # Lõi tải đa luồng, chia đoạn động, ghi đĩa trực tiếp
│   ├── media/            # Local HTTP Streaming Proxy & HLS Downloader
│   ├── scheduler/        # Hẹn giờ lập lịch tải thông minh
│   ├── server/           # REST APIs & Server-Sent Events (SSE)
│   └── nativemsg/        # Giao thức Native Messaging chuẩn Chrome/Edge
├── web/                  # Giao diện Desktop Web (HTML, CSS, JS)
├── extension/            # Tiện ích mở rộng Chrome/Edge (Manifest V3)
├── extension-firefox/    # Tiện ích mở rộng Mozilla Firefox (Gecko WebExtension)
└── ...
```

---

## 🚀 Hướng dẫn Cài đặt & Sử dụng

### 1. Cài đặt 1-Click (Khuyên dùng)
Bạn chỉ cần mở thư mục `dist\IDMM` và nhấp đúp chạy:
```cmd
install.bat
```
Script sẽ tự động đăng ký Native Messaging Host vào Windows Registry cho cả Chrome, Edge và Mozilla Firefox.

### 2. Chạy Ứng dụng Desktop
Chạy trực tiếp `dist\IDMM\idmm.exe` (hoặc `bin\idmm.exe`):
Cửa sổ ứng dụng độc lập Dark Mode sẽ tự động mở lên với đầy đủ tính năng: thêm link, hẹn giờ, chỉnh số luồng, giới hạn băng thông, xem video trực tiếp.

### 3. Sử dụng dòng lệnh (CLI)
Tải file với 16 luồng:
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://proof.ovh.net/files/10Mb.dat" -c 16
```
Tải và kích hoạt xem trực tiếp (Stream-as-you-download):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/movie.mp4" -stream
```
Tải video luồng trực tuyến m3u8 (HLS):
```powershell
.\dist\IDMM\idmm-cli.exe -url "https://example.com/playlist.m3u8" -o "video.mp4" -c 16
```

### 4. Cài đặt Browser Extension

#### A. Dành cho Google Chrome / Microsoft Edge:
1. Mở trình duyệt và truy cập trang quản lý extension:
   * **Chrome:** `chrome://extensions`
   * **Edge:** `edge://extensions`
2. Bật chế độ nhà phát triển (**Developer mode** ở góc trên bên phải).
3. Bấm **"Load unpacked"** (Tải tiện ích đã giải nén) và chọn thư mục `dist\IDMM\extension` (hoặc `D:\Personal\idmm\extension`).

#### B. Dành cho Mozilla Firefox:
1. Mở Firefox và truy cập vào địa chỉ:
   * `about:debugging#/runtime/this-firefox`
2. Bấm nút **"Load Temporary Add-on..."** (Tải tiện ích tạm thời).
3. Chọn file `manifest.json` trong thư mục `dist\IDMM\extension-firefox` (hoặc `D:\Personal\idmm\extension-firefox`).
4. Extension IDMM đã sẵn sàng hoạt động trên Firefox, tự động bắt link và kết nối trực tiếp với IDMM qua Native Messaging!

---

## 🛠️ Chạy Toàn Bộ Kiểm Thử (Unit & E2E Tests)

### Chạy Unit Tests:
```powershell
go test -v ./...
```
Toàn bộ các gói `internal/engine`, `internal/media`, `internal/scheduler`, `internal/server`, `internal/nativemsg` đạt **100% PASS**.

### Chạy End-to-End Test Suite:
```powershell
pwsh -File .\scripts\build_dist.ps1
# Chạy kịch bản E2E tự động xác thực: Dashboard -> Scheduler -> Range Proxy -> Checksum
```
