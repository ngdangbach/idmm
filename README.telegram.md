# 🚀 IDMM Telegram Media Downloader & Streamer
> **Nhánh:** `telegram-intergration` | **Ngôn ngữ:** Go (Golang 1.25+) | **Giao thức:** Telegram MTProto v2.0 (`gotd/td`)

Công cụ mở rộng cho **IDMM**, hoạt động như một Telegram Userbot Client hiệu năng cao giúp tự động tải, phân loại và đồng bộ toàn bộ video/hình ảnh từ các nhóm Telegram về ổ cứng máy tính trong thời gian thực cũng như quét lại toàn bộ lịch sử tin nhắn cũ.

---

## 🌟 Điểm Nổi Bật

* 📁 **Tự động phân loại theo ngày & tách riêng Ảnh / Video:**
  - File được lưu trữ theo cấu trúc:
    ```text
    D:/tmp/ (hoặc downloads/)
    └── 2026-10-07/
        ├── images/         # Toàn bộ hình ảnh (.jpg, .png)
        │   ├── photo_1001_20261007_123000.jpg
        │   └── ...
        └── videos/         # Toàn bộ video (.mp4, .mkv)
            ├── 1002_sample_video.mp4
            └── ...
    ```
* 📜 **Quét bù toàn bộ lịch sử tin nhắn cũ (Pagination History Downloader):**
  - Tự động cuộn ngược về quá khứ để tải hàng nghìn video/ảnh cũ từ trước đến nay.
  - Tùy chỉnh số lượng tin nhắn muốn quét (`history_limit: 1000` hoặc đặt `0` để tải toàn bộ từ lúc lập nhóm).
* ⚡ **Lắng nghe sự kiện thời gian thực (Real-time Event Listener):**
  - Cứ có tin nhắn media mới gửi vào nhóm, tool lập tức phát hiện và tải ngầm về máy ngay lập tức.
* 🛡️ **Hỗ trợ cả Nhóm Public & Nhóm Private:**
  - Nhận diện linh hoạt: Username (`@ten_nhom`), Link mời riêng tư (`https://t.me/+...`), hoặc Chat ID số (`-100...`).
* 👻 **Chế độ Khách xem ẩn danh (`auto_rejoin: false`):**
  - Với nhóm công khai, tool đọc và tải media dưới dạng "previewer" mà **không cần bấm Join vào nhóm**, hoàn toàn vô hình trước Admin.
* 🔄 **Tự động vào lại nhóm khi bị kick (`auto_rejoin: true`):**
  - Với nhóm private, nếu bị kick ra ngoài, tool tự động dùng link mời để join lại kèm cơ chế Exponential Backoff chống `FLOOD_WAIT`.
* 🔑 **Chỉ đăng nhập OTP một lần duy nhất:**
  - Tự động lưu Token phiên làm việc vào `session.telegram.json`. Các lần chạy sau tự động kết nối ngầm, không bao giờ phải nhập lại số điện thoại hay mật khẩu 2FA.
* 💾 **Chống tải trùng lặp (Resume & State Tracking):**
  - Toàn bộ file đã tải được ghi nhớ trong `state.telegram.json`. Tắt/mở tool tùy ý mà không sợ tải lại file cũ.

---

## 📦 Cấu Trúc Mã Nguồn

```text
idmm/
├── bin/
│   ├── idmm-telegram.exe         # File thực thi chuyên dụng cho Telegram
│   ├── idmm-cli.exe              # IDMM CLI tích hợp cờ -tg
│   ├── config.telegram.yaml      # File cấu hình hoạt động
│   └── session.telegram.json     # File lưu phiên đăng nhập Telegram
├── cmd/
│   ├── idmm-telegram/            # Mã nguồn entrypoint idmm-telegram
│   │   └── main.go
│   └── idmm-cli/                 # CLI chính hỗ trợ -telegram / -tg
│       └── main.go
├── internal/
│   └── telegram/
│       ├── config.go             # Nạp YAML & Smart Config Resolver
│       ├── auth.go               # Luồng xác thực CLI OTP & 2FA
│       ├── organizer.go          # Phân loại thư mục YYYY-MM-DD/images & videos
│       ├── organizer_test.go     # Unit tests cho organizer & invite parser
│       ├── state.go              # Quản lý state.telegram.json & resume
│       ├── state_test.go         # Unit tests cho persistence
│       ├── rejoiner.go           # Xử lý Auto Re-join & giải mã link invite
│       ├── downloader.go         # Streaming chunk downloader MTProto
│       └── service.go            # Điều phối kết nối, update dispatcher & pagination
└── config.telegram.yaml          # File cấu hình mẫu ở thư mục gốc
```

---

## ⚙️ Hướng Dẫn Cấu Hình (`config.telegram.yaml`)

File cấu hình mẫu nằm tại `config.telegram.yaml` (hoặc `bin/config.telegram.yaml`):

```yaml
telegram:
  api_id: 12345678
  api_hash: "0123456789abcdef0123456789abcdef"
  session_file: "session.telegram.json"
  phone: "" # Để trống để nhập tương tác từ Terminal ở lần đầu tiên

target:
  # Điền link invite (https://t.me/+...), username (@group), hoặc ID số
  identifier: "https://t.me/+AbCdEfGhIjK..."
  # Link mời dự phòng dùng để tự động vào lại khi bị kick (nếu nhóm là private)
  invite_link: "https://t.me/+AbCdEfGhIjK..."

settings:
  # Thư mục lưu file trên ổ đĩa (VD: "D:/tmp" hoặc "./downloads")
  download_dir: "./downloads"
  # Tự động join lại khi bị kick (false = chế độ xem khách cho nhóm public; true = tự join lại cho nhóm private)
  auto_rejoin: false
  # Thời gian chờ (giây) trước khi thử join lại nếu bị kick
  rejoin_retry_interval_sec: 60
  # Bật quét tải lịch sử tin nhắn cũ
  sync_history: true
  # Số lượng tin nhắn cũ cần quét (1000 = quét 1000 tin gần nhất; 0 = quét toàn bộ lịch sử từ ngày đầu)
  history_limit: 1000
  # Số luồng tải tối đa đồng thời
  max_concurrent_downloads: 3
```

---

## 🚀 Hướng Dẫn Sử Dụng

### Cách 1: Chạy công cụ độc lập `idmm-telegram.exe` (Khuyên dùng)
```powershell
# Chạy với file config mặc định
.\bin\idmm-telegram.exe

# Hoặc chỉ định trực tiếp nhóm mục tiêu qua dòng lệnh
.\bin\idmm-telegram.exe -target "https://t.me/+AbCdEfGhIjK..." -dir "D:/tmp"

# Hoặc chỉ định file config riêng
.\bin\idmm-telegram.exe -config "C:/path/to/my_config.yaml"
```

### Cách 2: Sử dụng IDMM CLI tích hợp (`idmm-cli.exe`)
```powershell
# Bật chế độ Telegram Downloader
.\bin\idmm-cli.exe -tg

# Ghi đè nhóm và link invite
.\bin\idmm-cli.exe -tg -tg-target "@my_group" -tg-invite "https://t.me/+inviteHash"
```

---

## 🛠️ Chạy Kiểm Thử Tự Động (Unit Tests)

Chạy kiểm thử toàn bộ module Telegram:
```powershell
go test -v ./internal/telegram
```

Kết quả:
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

## 📌 Các Lưu Ý Quan Trọng

1. **Lần chạy đầu tiên:** 
   - Terminal sẽ hỏi số điện thoại quốc tế (VD: `+84987654321`) và gửi mã OTP về ứng dụng Telegram của bạn.
   - Nhập OTP (và mật khẩu 2FA nếu tài khoản có cài đặt).
   - Sau khi hoàn tất, file `session.telegram.json` sẽ được tạo và lưu token vĩnh viễn.
2. **Cơ chế Smart Config Resolver:**
   - Dù bạn chạy lệnh từ thư mục gốc `idmm/` hay từ thư mục `bin/`, ứng dụng luôn tự động tìm kiếm file config và file session nằm cạnh file `.exe` hoặc trong thư mục hiện tại để nạp chính xác.
