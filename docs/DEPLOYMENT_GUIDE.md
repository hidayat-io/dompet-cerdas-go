# 🚀 Dokumentasi Proyek & Panduan Deployment — DompetCerdas Backend (Go)

Dokumen ini berisi panduan teknis lengkap mengenai arsitektur backend **DompetCerdas** berbasis **Golang (Go)**, alur kerja sistem, serta tata cara deployment *bare-metal* (super ringan tanpa Docker) ke server **`gcp-prau`**.

---

## 📌 Ringkasan Proyek

* **Nama Aplikasi**: DompetCerdas Backend API & Telegram Bot Engine
* **Bahasa & Framework**: Go 1.26+ (Framework **Gin Gonic**)
* **Database**: Google Cloud Firestore (Firebase Admin SDK v4)
* **Autentikasi**: Firebase Auth (Verifikasi Token JWT Bearer)
* **Kecerdasan Buatan (AI)**: Google Gemini SDK (`google.golang.org/genai` — Model `gemini-2.5-flash`)
* **Target Server**: Server GCP `gcp-prau` (IP: `35.209.238.160`, OS: Linux AMD64)
* **Arsitektur Deployment**: Bare-Metal Binary Statis via `systemd` + Reverse Proxy **Caddy** (HTTPS Otomatis)

---

## 🏗️ Arsitektur Sistem & Alur Kerja

Aplikasi backend ini merupakan **Modular Monolith** berbasis *Vertical Slice* yang menyatukan 3 fungsi utama ke dalam 1 file binary statis:

```
[ Frontend React (Firebase Hosting) ] ──> HTTP REST API ──┐
                                                           ├─> [ Go Binary (gcp-prau / systemd) ] ──> [ Firestore & Storage ]
[ Telegram App (@dompas_bot) ] ───────> Webhook POST ─────┤              │
                                                           └──────────────┼─> [ Google Gemini AI API ]
[ Internal Cron (Robfig Cron) ] ──────> Hourly Check ─────────────────────┘
```

1. **REST API Gateway**: Melayani request dari Web Frontend React 19 (seperti pendaftaran akun bersama, mutasi akun, analisis keuangan AI).
2. **Telegram Bot Engine**: Menangani Webhook pesan chat, input suara (Voice Note STT), foto struk belanja (Gemini Vision OCR), serta *callback button*.
3. **Internal Cron Scheduler**: Berjalan secara mandiri setiap jam (Asia/Jakarta) dengan perlindungan *Firestore Distributed Leader Lock* (`cron_locks/hourly_reminder`) agar aman dari pengiriman pengingat ganda.

---

## ⚡ Mengapa Deployment Bare-Metal (`systemd`) di `gcp-prau`?

Server `gcp-prau` memiliki spesifikasi 1GB RAM (GCP e2-micro). 
* **Dengan Docker**: Daemon Docker mengonsumsi ~50-100MB RAM tambahan.
* **Dengan Bare-Metal (`systemd`)**: Binary Go berjalan langsung di atas OS Linux.
  * 🚀 **Konsumsi RAM**: Hanya **~15–30 MB** (Super hemat, sisa RAM 500MB+ tetap lega).
  * 🚀 **Ukuran Binary**: Hanya **~15 MB** (Kompilasi statis `CGO_ENABLED=0`).
  * 🚀 **Cold Start**: **0 Detik** (Menyala 24/7 nonstop, Bot Telegram membalas instan <1s).

---

## 🛠️ Struktur Folder Proyek

```
dompet_cerdas_go/
├── cmd/
│   └── api/
│       └── main.go                 # Entrypoint utama server HTTP & Cron
├── deploy/                         # File Konfigurasi Deployment
│   ├── deploy.sh                   # Script otomatis cross-compile & upload ke gcp-prau
│   ├── dompet-cerdas.service       # File konfigurasi Systemd Service
│   └── Caddyfile                   # File konfigurasi Caddy (Reverse Proxy HTTPS)
├── docs/                           # Dokumentasi Teknis & Paritas
│   ├── DECISIONS.md                # Log Keputusan Arsitektur (ADR 1-14)
│   ├── MIGRATION_PLAN.md           # Rencana Migrasi 10 Fase
│   └── PARITY_CONTRACT.md          # Kontrak Paritas Parser & Regex
├── internal/
│   ├── config/                     # Pemetaan & Validasi Environment Variable (.env)
│   ├── domain/                     # Struct Data Firestore (Account, Transaction, dll)
│   ├── middleware/                 # Firebase Auth JWT, CORS Allowlist, Logger
│   ├── modules/                    # VERTICAL SLICES LOGIKA BISNIS
│   │   ├── account/                # Manajemen Akun & Resumable Shared Workspace Job
│   │   ├── advisor/                # Web AI Analysis & Rolling 24h Quota Tracker
│   │   ├── health/                 # Health Check Endpoint (/api/v1/health)
│   │   ├── reminder/               # Scheduler Pengingat Jam/Harian via Leader Lock
│   │   ├── telegram/               # Webhook Bot, NLU Engine, & Formatter
│   │   └── transaction/            # Parser Transaksi Hybrid & Query Builder
│   └── shared/
│       ├── datetime/               # Timezone Asia/Jakarta & Resolusi Penanggalan
│       ├── db/                     # Inisialisasi Firebase & Firestore Client
│       ├── gemini/                 # Client SDK Google Gemini AI
│       ├── money/                  # Parsing Nominal & Formatting Rupiah (Rp 25k)
│       └── ratelimit/              # Rate Limiter Memory & Anti-Spam
├── testdata/parity/                # 8 File Fixtures (299 Kasus Uji Riil Node.js)
├── Makefile                        # Command Shortcuts (build, test, verify, deploy)
└── README.md                       # Ringkasan Proyek
```

---

## 🚀 Panduan Deployment ke `gcp-prau` (Langkah demi Langkah)

### Prasyarat di Komputer Lokal
1. Pastikan Anda dapat melakukan SSH ke server tanpa password: `ssh gcp-prau`.
2. Pastikan file `.env` dan `service-account.json` (kredensial Firebase) ada di folder `dompet_cerdas_go`.

---

### Langkah 1: Jalankan Deployment Otomatis dari Lokal

Di terminal lokal Anda, jalankan perintah berikut:

```bash
cd ~/Dev-Labs/dompet_cerdas_go

# Mengompilasi binary Linux AMD64 lalu mengunggahnya ke gcp-prau
make deploy
# Atau jalankan script langsung: ./deploy/deploy.sh
```

Script ini akan:
1. Mengompilasi kode Go menjadi binary Linux AMD64 statis (`bin/dompet-cerdas-go`).
2. Mengunggah binary, file `.env`, `service-account.json`, `dompet-cerdas.service`, dan `Caddyfile` ke server `gcp-prau` di direktori `/home/mthidayat/dompet-cerdas-backend/`.

---

### Langkah 2: Setup Systemd Service di Server `gcp-prau` (Hanya Sekali)

Login ke server `gcp-prau`:
```bash
ssh gcp-prau
```

Di dalam server, daftarkan service ke OS agar backend Go otomatis menyala saat server booting:
```bash
# Copy file service ke folder systemd
sudo cp /home/mthidayat/dompet-cerdas-backend/dompet-cerdas.service /etc/systemd/system/

# Reload systemd daemon
sudo systemctl daemon-reload

# Aktifkan dan jalankan service
sudo systemctl enable --now dompet-cerdas

# Cek status service
sudo systemctl status dompet-cerdas
```

---

### Langkah 3: Install Caddy & Setup HTTPS (Hanya Sekali)

Di dalam server `gcp-prau`, jalankan perintah berikut untuk menginstal **Caddy** (WebServer yang otomatis mengurus sertifikat SSL Let's Encrypt):

```bash
# Install Caddy di Debian/Ubuntu
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update && sudo apt install caddy

# Salin Caddyfile yang sudah kita siapkan
sudo cp /home/mthidayat/dompet-cerdas-backend/Caddyfile /etc/caddy/Caddyfile

# Reload Caddy
sudo systemctl reload caddy
```

---

### Langkah 4: Set Webhook Telegram ke Server Baru

Setelah HTTPS Caddy menyala, daftarkan URL webhook baru ke Telegram Bot API:

```bash
curl -X POST "https://api.telegram.org/bot<TELEGRAM_BOT_TOKEN>/setWebhook" \
  -d "url=https://api.dompas.indoomega.my.id/api/v1/telegram/webhook"
```

---

## 📊 Perintah Manajemen & Monitoring di Server `gcp-prau`

```bash
# Cek log aplikasi secara real-time
sudo journalctl -u dompet-cerdas -f

# Restart backend Go
sudo systemctl restart dompet-cerdas

# Stop backend Go
sudo systemctl stop dompet-cerdas

# Cek penggunaan RAM & CPU
top -p $(pgrep dompet-cerdas)
```

---

## 🧪 Verifikasi & Smoke Test Pasca Deploy

Setelah deploy, jalankan verifikasi berikut:

1. **Health Check Endpoint**:
   ```bash
   curl https://api.dompas.indoomega.my.id/api/v1/health
   # Respons diharapkan: {"success":true,"message":"Server is healthy", ...}
   ```
2. **Uji Coba Bot Telegram**:
   - Kirim chat `/start` ke `@dompas_bot` di Telegram. Bot akan membalas instan (<1 detik).
   - Kirim foto struk belanja untuk menguji Gemini Vision OCR.
