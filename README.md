# 🤖 DISV - Discord Multifunctional Bot (Go & Python)

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version" />
  <img src="https://img.shields.io/badge/Discord.go-v0.28+-5865F2?style=for-the-badge&logo=discord&logoColor=white" alt="DiscordGo" />
  <img src="https://img.shields.io/badge/MySQL-8.0+-4479A1?style=for-the-badge&logo=mysql&logoColor=white" alt="MySQL" />
  <img src="https://img.shields.io/badge/FFmpeg-Supported-007808?style=for-the-badge&logo=ffmpeg&logoColor=white" alt="FFmpeg" />
  <img src="https://img.shields.io/badge/Python-3.10+-3776AB?style=for-the-badge&logo=python&logoColor=white" alt="Python Version" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License" />
</p>

**DISV** là bot Discord đa năng thế hệ mới, được thiết kế với hiệu năng vượt trội bằng **Golang** (100% Native DiscordGo & MySQL) cùng phiên bản tiền nhiệm bằng **Python**. Bot tích hợp hệ thống tải TikTok tự động nén theo cấp độ server, Ticket hỗ trợ chuyên nghiệp, Chat AI Google Gemini, quản trị tự động, chống Spam, Leveling XP và bảng điều khiển trực quan.

---

## 🌟 Tính Năng Nổi Bật

### 1. 🎬 TikTok Downloader & Nén Video Thông Minh
- Tự động nhận diện mọi liên kết TikTok trong kênh chat (`vt.tiktok.com`, `vm.tiktok.com`, `tiktok.com/@user/video/...`).
- **Tự động kiểm tra giới hạn upload của Server (`filesize_limit`)**:
  - Server thường: Tối đa **10MB**.
  - Server Nitro Boost Tier 2: Tối đa **50MB**.
  - Server Nitro Boost Tier 3: Tối đa **100MB**.
- Nếu kích thước video vượt quá mức cho phép của Guild, hệ thống sẽ tự động gọi **FFmpeg** tính toán bitrate và nén video xuống đúng ngưỡng dung lượng mà vẫn giữ độ sắc nét cao nhất.
- Hỗ trợ tải bài đăng dạng **Slide ảnh** kèm trích xuất nhạc nền (`audio.mp3`).

### 2. 🎫 Hệ Thống Ticket Hỗ Trợ Toàn Diện
- Lệnh `/ticket send` gửi panel hỗ trợ với Button & Modal nhập lý do mở ticket.
- Tự động phân quyền kênh riêng tư: chỉ người tạo và đội ngũ quản trị viên mới xem được.
- Đóng ticket trực tiếp qua nút bấm, tự động lưu lịch sử trao đổi vào **MySQL**.

### 3. 🤖 Chat AI (Google Gemini)
- Tích hợp mô hình ngôn ngữ lớn **Google Gemini**.
- Tùy chỉnh tính cách và phong cách phản hồi thông qua tệp dữ liệu ngữ cảnh `train.txt`.
- Tự động trả lời khi được ping hoặc trong các kênh được bật tính năng Chat AI.

### 4. 🛡️ Quản Trị & Anti-Spam
- Hệ thống **Anti-Spam** theo dõi tần suất gửi tin nhắn trong thời gian thực, tự động cảnh báo hoặc timeout người dùng vi phạm.
- Bộ lệnh quản trị Slash Command chuẩn mực: `/mute`, `/unmute`, `/kick`, `/ban`, `/clear`.
- Ghi nhận lịch sử vi phạm chi tiết vào cơ sở dữ liệu MySQL.

### 5. 🏆 Hệ Thống Leveling & XP
- Tích lũy kinh nghiệm (XP) và thăng cấp tự động khi tham gia trò chuyện.
- Cơ chế cooldown chống cày cấp ảo (spam exp).
- Lệnh `/rank` hiển thị cấp độ, XP hiện tại và thanh tiến trình.

### 6. 🎭 Auto-roles (Tự Động Gán Vai Trò)
- Tự động cấp Role cho thành viên mới khi vừa tham gia server (`GuildMemberAdd`).
- Cấu hình linh hoạt qua lệnh `/autorole set` và kiểm tra qua `/autorole show`.

### 7. 📊 Dashboard Cấu Hình Kênh Trực Quan
- Lệnh `/setting` mở giao diện tương tác (Buttons/Select) cho phép Admin bật/tắt từng tính năng độc lập cho từng kênh (TikTok, Chat AI, Leveling,...).

---

## 📂 Cấu Trúc Thư Mục Dự Án

```plaintext
botdis/
├── go/                            # Mã nguồn Golang chính (100% Core & Production)
│   ├── cmd/
│   │   └── bot/
│   │       └── main.go            # Điểm khởi chạy ứng dụng Go
│   ├── config/
│   │   └── config.go              # Quản lý & nạp cấu hình hệ thống
│   ├── internal/
│   │   ├── discord/
│   │   │   └── router.go          # Router điều phối Slash Commands, Modals, Buttons
│   │   ├── modules/               # Các module chức năng độc lập
│   │   │   ├── autoroles/         # Tự động gán role thành viên mới
│   │   │   ├── chatai/            # Chat AI Google Gemini & train.txt
│   │   │   ├── dashboard/         # Bảng điều khiển cấu hình /setting
│   │   │   ├── leveling/          # Cấp độ & kinh nghiệm /rank
│   │   │   ├── moderation/        # Lệnh xử phạt & Anti-spam
│   │   │   ├── ticket/            # Hệ thống Ticket hỗ trợ khách hàng
│   │   │   ├── tiktok/            # Tải & nén video TikTok bằng FFmpeg
│   │   │   └── utility/           # Lệnh tiện ích /ping, /botinfo
│   │   └── storage/
│   │       └── mysql.go           # Kết nối & Auto-migration cơ sở dữ liệu MySQL
│   ├── go.mod                     # Quản lý Go dependencies
│   └── go.sum
├── py/                            # Mã nguồn Python (Phiên bản tham chiếu gốc)
│   ├── cogs/                      # Các cogs discord.py
│   ├── utils/                     # Tiện ích bổ trợ (Database, Anti-spam)
│   ├── main.py                    # File khởi chạy bản Python
│   └── requirements.txt           # Thư viện Python
├── videotiktok/                   # Thư mục tạm thời xử lý video/ảnh
├── config.example.json            # File mẫu cấu hình an toàn
├── train.txt                      # Dữ liệu ngữ cảnh đào tạo cho Chat AI
├── .gitignore                     # Cấu hình loại trừ file nhạy cảm & build
└── README.md                      # Hướng dẫn sử dụng
```

---

## ⚙️ Yêu Cầu Hệ Thống

1. **Golang**: Phiên bản `1.22` trở lên.
2. **MySQL Server**: Phiên bản `8.0+` hoặc MariaDB tương đương.
3. **FFmpeg**: Đã cài đặt và thêm vào biến môi trường hệ thống (`PATH`) để hỗ trợ chuyển mã video.
4. **Python**: `3.10+` (chỉ cần nếu bạn muốn chạy phiên bản Python cũ trong thư mục `py/`).

---

## 🚀 Hướng Dẫn Cài Đặt & Khởi Chạy

### 1. Sao chép mã nguồn
```bash
git clone https://github.com/ngdaison/disv.git
cd disv
```

### 2. Thiết lập cơ sở dữ liệu MySQL
Tạo cơ sở dữ liệu mới trong MySQL:
```sql
CREATE DATABASE botdis CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```
*(Hệ thống sẽ tự động tạo bảng và migrate cấu trúc khi khởi chạy bot lần đầu).*

### 3. Cấu hình Bot
Sao chép tệp cấu hình mẫu:
```bash
cp config.example.json config.json
```
Mở `config.json` và điền đầy đủ các thông số:
```json
{
    "bot_token": "DISCORD_BOT_TOKEN_CUA_BAN",
    "rapid_api_key": "RAPID_API_KEY_TIKTOK",
    "ai_api_key": "GOOGLE_GEMINI_API_KEY",
    "chatai_token": "",
    "bot_prefix": "/",
    "status_message": "Đang quản lý server",
    "max_video_size_mb": 10,
    "mysql": {
        "host": "127.0.0.1",
        "port": 3306,
        "user": "root",
        "password": "mat_khau_mysql",
        "database": "botdis"
    }
}
```

> ⚠️ **Lưu ý bảo mật:** Tuyệt đối không commit hoặc chia sẻ tệp `config.json` chứa token thật lên GitHub!

---

### 4. Khởi chạy Bot phiên bản Golang (Khuyên dùng)

#### Chạy trực tiếp từ mã nguồn:
```bash
cd go
go run ./cmd/bot
```

#### Biên dịch thành file thực thi độc lập:
- **Trên Windows:**
  ```powershell
  cd go
  go build -o botdis.exe ./cmd/bot
  .\botdis.exe
  ```
- **Trên Linux / macOS:**
  ```bash
  cd go
  go build -o botdis ./cmd/bot
  ./botdis
  ```

---

### 5. Khởi chạy phiên bản Python (Dự phòng)
Nếu muốn sử dụng phiên bản Python truyền thống:
```bash
cd py
pip install -r requirements.txt
python main.py
```

---

## 📜 Danh Sách Lệnh (Slash Commands)

| Lệnh | Mô tả | Quyền hạn |
| :--- | :--- | :--- |
| `/ticket send [channel]` | Gửi bảng điều khiển tạo Ticket vào kênh được chọn | Quản trị viên |
| `/setting` | Mở bảng điều khiển cài đặt tính năng cho kênh/server | Quản trị viên |
| `/autorole set <role>` | Thiết lập vai trò tự động trao cho người mới | Quản trị viên |
| `/autorole show` | Xem cấu hình vai trò tự động hiện tại | Mọi người |
| `/rank [user]` | Kiểm tra cấp độ và thanh điểm kinh nghiệm (XP) | Mọi người |
| `/clear [amount]` | Xóa nhanh số lượng tin nhắn trong kênh (tối đa 100) | Quản lý tin nhắn |
| `/mute <user> [minutes] [reason]` | Khóa mõm (Timeout) thành viên | Quản lý thành viên |
| `/unmute <user>` | Hủy Timeout cho thành viên | Quản lý thành viên |
| `/kick <user> [reason]` | Đuổi thành viên ra khỏi máy chủ | Đuổi thành viên |
| `/ban <user> [reason]` | Cấm thành viên vĩnh viễn khỏi máy chủ | Cấm thành viên |
| `/ping` | Đo độ trễ phản hồi của Bot đến Discord Gateway | Mọi người |
| `/botinfo` | Xem thông tin hệ thống (RAM, Go Runtime, Uptime) | Mọi người |

---

## 🤝 Đóng Góp (Contributing)
Mọi đóng góp nhằm cải thiện và phát triển thêm tính năng cho bot đều được hoan nghênh:
1. Fork dự án.
2. Tạo nhánh tính năng (`git checkout -b feature/tinh-nang-moi`).
3. Commit các thay đổi (`git commit -m 'feat: them tinh nang moi'`).
4. Đẩy lên nhánh của bạn (`git push origin feature/tinh-nang-moi`).
5. Tạo một **Pull Request**.

---

## 📄 Giấy Phép (License)
Dự án được phân phối dưới giấy phép **MIT License**. Xem thêm chi tiết tại tệp [LICENSE](LICENSE).