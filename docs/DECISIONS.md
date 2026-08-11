# Architecture Decision Records (ADR) Log

Dokumen ini mencatat keputusan arsitektur penting yang diambil selama proses menulis ulang (rewrite) backend **DompetCerdas** dari Node.js/Firebase Cloud Functions ke Go self-hosted.

## Ringkasan Keputusan

| ADR | Judul | Status | Dampak |
|---|---|---|---|
| [ADR-001](#adr-001-migrasi-dari-firebase-cloud-functions-ke-go-self-hosted) | Migrasi dari Firebase Cloud Functions ke Go self-hosted | Accepted | Latency Telegram bot terpangkas, konsumsi memori rendah, kontrol infrastruktur penuh. |
| [ADR-002](#adr-002-gemini-sdk-langsung-drop-9router) | Gemini SDK langsung, drop 9Router | Accepted | Menggunakan `google.golang.org/genai` secara langsung, menghapus proxy 9Router. |
| [ADR-003](#adr-003-gemini-vision-menggantikan-tesseract-ocr-lokal) | Gemini Vision menggantikan Tesseract OCR lokal | Accepted | Menghapus Tesseract.js WASM / CGO dependencies, performa receipt processing meningkat. |
| [ADR-004](#adr-004-token-counting-nyata-menggantikan-estimasi-length--4) | Token counting nyata menggantikan estimasi `length / 4` | Accepted | Kuota harian akurat menggunakan `UsageMetadata`, berdampak pada limit harian pengguna. |
| [ADR-005](#adr-005-image-processing-pure-go-disintegrationimaging) | Image processing pure Go (`disintegration/imaging`) | Accepted | Tetap menggunakan `CGO_ENABLED=0` untuk build statis, menggantikan library `sharp` (libvips). |
| [ADR-006](#adr-006-rate-limiter-in-memory-di-belakang-interface) | Rate limiter in-memory di belakang interface | Accepted | Rate limiting 5 req/10s per user berjalan konsisten di single container tanpa Redis overhead. |
| [ADR-007](#adr-007-shareexistingaccount--resumable-job--copy-then-soft-cutover) | `shareExistingAccount` — resumable job + copy-then-soft-cutover | Accepted | Mengubah data sharing menjadi multi-phase idempotent job untuk mencegah data loss akibat kegagalan Firestore batch. |
| [ADR-008](#adr-008-urutan-cutover--web-callables-dulu-telegram-bot-terakhir) | Urutan cutover — web callables dulu, Telegram bot terakhir | Accepted | Meminimalkan risiko switchover dengan migrasi endpoint web callable secara bertahap. |
| [ADR-009](#adr-009-firestore-leader-lock-untuk-cron-selama-coexistence) | Firestore leader lock untuk cron selama coexistence | Accepted | Mencegah double-reminders menggunakan locking mechanism di Firestore `cron_locks/hourly_reminder`. |
| [ADR-010](#adr-010-path-resolution-firestore-tetap-3-varian) | Path resolution Firestore tetap 3 varian | Accepted | Mendukung skema legacy, private, dan shared accounts tanpa melakukan migrasi skema data produksi. |
| [ADR-011](#adr-011-auto-save-gate-dipertahankan--wajib-audit-logging) | Auto-save gate dipertahankan + wajib audit logging | Accepted | Menjaga UX Telegram bot auto-save, menambahkan structured logging untuk deteksi false positives. |
| [ADR-012](#adr-012-bug-last_month-diperbaiki-bukan-direplikasi) | Bug `last_month` diperbaiki, bukan direplikasi | Accepted | Memperbaiki bug penanggalan akhir bulan di Go menggunakan kalkulasi kalender yang presisi. |
| [ADR-013](#adr-013-pergeseran-satu-hari-this_month-dan-custom_month-diperbaiki) | Pergeseran satu hari `this_month` dan `custom_month` diperbaiki | Accepted | Menghapus pergeseran UTC yang membuat batas bulan mundur satu hari. |
| [ADR-014](#adr-014-wart-pencocokan-substring-query_keywords-dipertahankan) | Wart pencocokan substring `QUERY_KEYWORDS` dipertahankan | Accepted | "bayar listrik 350rb" tertolak diam-diam di produksi; dipertahankan dan di-pin pengujian sampai ada keputusan produk. |
| [ADR-015](#adr-015-escapemarkdown-dipersempit-ke-4-karakter-spesial-v1-bukan-direplikasi-dari-markdownv2) | `EscapeMarkdown` dipersempit ke 4 karakter spesial V1, bukan direplikasi dari MarkdownV2 | Accepted | Menghilangkan backslash mentah yang bocor ke setiap pesan konfirmasi transaksi (mis. `"Rp150\.000"`) karena escaper lama memakai character class V2 sedangkan parse_mode produksi tetap V1. |

---

## ADR-001: Migrasi dari Firebase Cloud Functions ke Go self-hosted

### Status
Accepted

### Konteks
Pada backend lama yang berbasis Firebase Cloud Functions (Node.js 22 + TypeScript), aplikasi mengalami kendala latency akibat *cold starts*. Bot Telegram membutuhkan respon cepat (<2-3 detik) agar tidak dianggap mati oleh server Telegram, sedangkan *cold start* Cloud Functions sering kali mendorong respon pertama hingga 5-8 detik. 

Selain itu, biaya berbasis pemanggilan (*per-invocation billing*) meningkat seiring pertumbuhan trafik bot. Penjadwalan tugas (*cron jobs*) via Cloud Scheduler dikelola sebagai resource terpisah yang mempersulit debugging lokal. Node.js yang bersifat *single-thread* juga membatasi skalabilitas untuk menangani tugas AI dan pemrosesan gambar secara konkuren.

### Keputusan
Menulis ulang backend DompetCerdas ke dalam bahasa Go sebagai satu *monolithic binary* dengan struktur *Modular Monolith + Vertical Slice*. Aplikasi ini dideploy menggunakan Docker di VPS mandiri di belakang reverse proxy Nginx/Traefik.

### Konsekuensi
- **Kelebihan**: Masalah *cold start* teratasi sepenuhnya. Konsumsi memori idle berkurang drastis menjadi <30MB. Dukungan konkurensi bawaan via *goroutines* mempermudah tugas paralel. Log sistem terkonsolidasi dalam satu aliran (*single log stream*). Keadaan lokal proses (*process-local state* seperti *in-memory rate limiter*) kini dapat bertahan lama secara nyata.
- **Kekurangan**: Tanggung jawab terkait pemeliharaan infrastruktur seperti *uptime*, konfigurasi TLS/SSL, prosedur deployment, dan pembaruan sistem operasi (*OS patching*) kini beralih sepenuhnya ke tim internal, tidak lagi diotomatisasi oleh Google Cloud. Status lokal proses yang bertahan lama memicu risiko ketidakselarasan data (*staleness risk*) jika tidak dikelola dengan benar.

---

## ADR-002: Gemini SDK langsung, drop 9Router

### Status
Accepted (menggantikan rancangan draf PRD yang menyarankan penggunaan 9Router)

### Konteks
Kode backend lama memiliki pemisahan lapisan AI yang kurang konsisten. Komponen `routerService.ts` (138 LOC) merupakan klien HTTP OpenAI-compatible berbasis *native* `fetch` yang diarahkan ke proxy mandiri `https://9r.indoomega.my.id/v1` dengan model `kang-coding`. Komponen ini menerapkan mekanisme 2 kali *retry* pada error 429/5xx, serta sengaja mengabaikan parameter `max_tokens` untuk menghindari bug total token pada proxy tersebut. 

`routerService.ts` menangani NLU (*Natural Language Understanding*), konversi teks nota belanja menjadi JSON, klasifikasi kategori, dan fitur asisten keuangan (*financial advisor*). Di sisi lain, `geminiService.ts` menggunakan SDK resmi `@google/genai` dengan model `gemini-2.5-flash` khusus untuk transkripsi audio karena membutuhkan input biner audio secara langsung. Draf awal PRD (`docs/PRD_REWRITE_GO.md` pada repo lama) sempat menyarankan mempertahankan arsitektur 9Router.

### Keputusan
Menggunakan library resmi `google.golang.org/genai` secara langsung untuk seluruh fitur berbasis LLM dan menghentikan penggunaan proxy 9Router. Layanan `routerService.ts` sepenuhnya dihapus dalam porting ke Go.

### Konsekuensi
- **Kelebihan**: Hanya ada satu jenis AI client SDK yang dipelihara. Kita mendapatkan akses langsung ke data penggunaan token yang akurat via `UsageMetadata` (menunjang ADR-004). Mengurangi ketergantungan pada infrastruktur eksternal pihak ketiga serta menghindari bug 429 pada proxy tersebut.
- **Kekurangan**: Kehilangan fleksibilitas pemindahan model secara instan (*model-swap*) yang sebelumnya disediakan oleh proxy. Kode backend Go kini terikat erat dengan spesifikasi API dari SDK Gemini. Logika penanganan batas limit API (*retry* dan *backoff*) yang sebelumnya ditangani oleh proxy kini harus diimplementasikan secara manual di backend Go.

---

## ADR-003: Gemini Vision menggantikan Tesseract OCR lokal

### Status
Accepted

### Alternatif yang Dipertimbangkan
- **Alternatif A**: Binding CGO `gosseract`. Memerlukan variabel lingkungan `CGO_ENABLED=1`, mencegah pembuatan *static binary* Go yang murni, serta memperbesar ukuran Docker image hingga 200-400MB.
- **Alternatif B**: Menjalankan executable CLI `tesseract` melalui perintah shell (*subprocess*). Menjaga binary Go tetap statis namun memicu overhead performa tinggi karena membuat proses sistem baru untuk setiap nota belanja yang diproses.
- **Alternatif C**: Memanfaatkan kemampuan multimodal Gemini Vision langsung pada berkas gambar nota. Menghapus seluruh lapisan OCR Tesseract dari backend.
- **Alternatif D**: Menjalankan OCR Tesseract sebagai container terpisah (*sidecar container*).

### Konteks
Alur pemrosesan nota belanja lama membutuhkan dua tahap. Pertama, `ocrService.ts` (36 LOC) menjalankan Tesseract.js WASM menggunakan fungsi `createWorker('ind+eng')` setelah memproses gambar menggunakan library `sharp` (resize ke lebar maksimal 1600px, diubah ke *grayscale*, dinormalisasi, dan disimpan sebagai JPEG kualitas 80). Berkas data bahasa `ind.traineddata` dan `eng.traineddata` disimpan langsung di dalam repositori. Kedua, teks mentah hasil OCR dikirim ke LLM untuk diekstrak menjadi struktur JSON. 

Sebagai catatan, endpoint web callable `scanReceipt` sudah melompati Tesseract dan memanggil `analyzeReceipt` langsung menggunakan buffer gambar terkompresi.

### Keputusan
Menggunakan **Alternatif C**. Gambar yang telah di-resize langsung dikirimkan ke model Gemini Vision untuk diekstrak langsung ke format terstruktur JSON tanpa melalui pembacaan teks OCR perantara.

### Konsekuensi
- **Kelebihan**: Pengaturan kompilasi Go tetap menggunakan `CGO_ENABLED=0` untuk menghasilkan *single static binary*. Berkas bahasa traineddata Tesseract dapat dihapus dari repositori sehingga ukuran Docker image menyusut tajam. Mengurangi jumlah panggilan API eksternal dari dua tahap (OCR + LLM) menjadi satu tahap langsung. Akurasi pengenalan teks pada nota belanja dengan cetakan thermal bersuhu rendah (sering kali memudar dan gagal dibaca Tesseract `ind`) terbukti meningkat signifikan.
- **Kekurangan**: Karakteristik kesalahan pengenalan (*accuracy drift*) akan bergeser. Karena ini merupakan aplikasi keuangan, kesalahan pembacaan `totalAmount` akan langsung mencatat nominal transaksi yang keliru. 
- **Persyaratan Validasi**: Sebelum migrasi penuh dijalankan (*cutover*), tim wajib menjalankan kumpulan data gambar nota belanja asli melalui jalur lama dan Gemini Vision untuk membandingkan output `merchant`, `totalAmount`, `date`, dan tingkat keyakinan `confidence` secara berdampingan (*golden test suite*). Pengiriman gambar langsung ke Google Cloud API juga menambah cakupan paparan data sensitif pengguna (*data-exposure delta*) dibanding pemrosesan teks lokal sebelumnya.

---

## ADR-004: Token counting nyata menggantikan estimasi `length / 4`

### Status
Accepted (membutuhkan keputusan produk terkait penyesuaian limit harian)

### Konteks
Pada kode lama, khususnya berkas `geminiService.ts` baris 254-256, penghitungan kuota token dihitung menggunakan rumus pendekatan panjang karakter:
```ts
const promptTokens = Math.ceil(dataPrompt.length / 4);
const candidateTokens = Math.ceil(response.length / 4);
const totalTokens = promptTokens + candidateTokens;
```
Rumus `length / 4` merupakan teknik estimasi kasar, bukan penggunaan token yang sesungguhnya di sisi model AI. Kebijakan kuota harian sebesar 30.000 token per hari yang didefinisikan dalam `webAnalysisService.ts` selama ini ditegakkan berdasarkan nilai estimasi fiktif tersebut.

### Keputusan
Menggunakan data penggunaan token riil dari objek respon `UsageMetadata` yang disediakan oleh SDK Gemini resmi di backend Go.

### Konsekuensi
- **Kelebihan**: Penegakan kuota harian menjadi akurat, adil, dan mencerminkan biaya operasional API yang sesungguhnya.
- **Kekurangan**: Batas efektif kuota harian bagi pengguna akan mengalami pergeseran. Bahasa Indonesia memiliki karakteristik tokenisasi yang menghasilkan jumlah token lebih tinggi dibandingkan rasio estimasi 1 token per 4 karakter. Pengguna kemungkinan besar akan mencapai batas harian 30.000 token lebih cepat daripada sistem lama.
- **Tindak Lanjut**: Tim produk harus memutuskan apakah akan menyesuaikan batas harian (*re-tuning limit*) agar setara dengan pola konsumsi riil pengguna, atau menerima dampak pembatasan yang lebih ketat ini. Direkomendasikan untuk melakukan pencatatan (*logging*) perbandingan nilai token riil vs nilai estimasi selama masa coexistence sebelum mengubah batas kuota secara resmi. Keputusan ini beririsan langsung dengan Rencana Migrasi Fase 5 (`docs/MIGRATION_PLAN.md`).

---

## ADR-005: Image processing pure Go (`disintegration/imaging`)

### Status
Accepted

### Konteks
Backend lama menggunakan library `sharp` untuk kompresi dan prapemrosesan gambar. Library `sharp` bergantung pada pustaka sistem C++ native (`libvips`). Untuk replikasi di Go, terdapat beberapa opsi populer seperti `disintegration/imaging` (murni bahasa Go), `h2non/bimg` (menggunakan CGO dan libvips), atau `govips` (menggunakan CGO dan libvips). Logika pengubahan warna ke abu-abu (*grayscale*) dan normalisasi kontras sebelumnya diperlukan semata-mata untuk meningkatkan akurasi engine Tesseract OCR.

### Keputusan
Menggunakan library `github.com/disintegration/imaging` yang ditulis murni dalam bahasa Go.

### Konsekuensi
- **Kelebihan**: Mempertahankan kompilasi tanpa CGO (`CGO_ENABLED=0`), sehingga binary hasil kompilasi tetap mandiri (*static binary*) dan ukuran container Docker tetap minimal. Seiring dihapusnya Tesseract OCR (ADR-003), logika pemrosesan gambar disederhanakan hanya mencakup operasi ubah ukuran (*resize*) dan kompresi enkoder JPEG sebelum diunggah ke Gemini Vision API.
- **Kekurangan**: Performa kompresi gambar berbasis pustaka murni Go memiliki kecepatan komputasi yang lebih lambat dibandingkan pemrosesan berbasis assembly libvips/C++. Namun, karena beban kerja aplikasi hanya memproses nota secara sekuensial per permintaan pengguna, perbedaan latensi milidetik ini tidak berdampak buruk pada performa aplikasi secara keseluruhan.

---

## ADR-006: Rate limiter in-memory di belakang interface

### Status
Accepted

### Konteks
Pada backend Cloud Functions lama, sistem pembatasan laju pesan (*rate limiting*) menggunakan struktur data `Map` dalam memori lokal pada berkas `bot/index.ts`. Aturan yang diterapkan adalah membatasi maksimal 5 permintaan per 10 detik per pengguna Telegram, serta melakukan supresi pesan duplikat dalam kurun waktu 5 detik. Karena Cloud Functions bersifat *stateless* dan instansinya sering kali mati dan hidup kembali, struktur data `Map` ini sering terhapus. Akibatnya, *rate limiter* tersebut jarang bekerja secara efektif di produksi. Pustaka `Map` lama juga tidak memiliki mekanisme pembersihan data kedaluwarsa (*garbage collection/eviction*) sehingga rentan mengalami kebocoran memori jika instansi hidup terlalu lama.

### Keputusan
Mengimplementasikan struktur data *rate limiter* dalam memori (*in-memory*) yang dibungkus di belakang sebuah *interface* Go yang bersih:
```go
type Limiter interface {
    Allow(key string) (allowed bool, retryAfter time.Duration)
    IsDuplicate(key, payload string) bool
}
```
Layanan ini dilengkapi dengan *janitor goroutine* yang berjalan secara berkala untuk menghapus data pengguna yang sudah melewati masa kadaluwarsa limitasi.

### Konsekuensi
- **Kelebihan**: Implementasi tidak membutuhkan infrastruktur tambahan (seperti Redis) sehingga menghemat biaya sewa VPS dan kompleksitas operasi. Tidak ada pembacaan ke database Firestore untuk setiap pesan Telegram yang masuk. Pilihan yang sangat efisien untuk arsitektur *single container*.
- **Kekurangan**: Logika ini tidak mendukung skalabilitas jika aplikasi dijalankan di lebih dari satu instansi kontainer (*multiple replicas*). Pilihan penggunaan *interface* menjamin bahwa jika di masa depan arsitektur berkembang membutuhkan Redis, tim dapat mengganti implementasinya tanpa mengubah kode di sisi pemanggil. Adanya *in-memory rate limiter* yang benar-benar persisten pada proses Go yang hidup lama membuat pembatasan ini kini bekerja secara nyata di produksi, yang berarti beberapa pengguna yang terbiasa mengirim pesan beruntun mungkin akan mulai terkena limitasi.

---

## ADR-007: `shareExistingAccount` — resumable job + copy-then-soft-cutover

### Status
Accepted

### Konteks
Logika pembagian akun bersama (`shareExistingAccount`) merupakan area dengan tingkat risiko tertinggi di backend lama. Proses migrasi akun privat menjadi akun bersama terbagi menjadi 3 fase non-atomik:
1. Menulis dokumen baru di `sharedAccounts/{sid}`, membuat dokumen anggota pemilik (*owner member*), memperbarui status akun pengguna, serta mengubah field `activeAccountId`.
2. Menyalin dokumen dari 6 subcollection (`categories`, `transactions`, `plans`, `budgets`, `simulations`, `debts`) dari path asal `users/{uid}/accounts/{aid}/*` menuju path tujuan `sharedAccounts/{sid}/*` dalam batch maksimal 400 dokumen per eksekusi.
3. Menghapus seluruh dokumen asli pada 6 subcollection asal dalam batch 400 dokumen.

Jika proses gagal di tengah jalan (misal pada fase 2), penanganan error lama hanya menghapus dokumen `sharedAccounts` dan dokumen anggota pemilik di fase 1. Sistem tidak menghapus dokumen subcollection yang sudah terlanjur disalin pada fase 2, dan tidak memulihkan data asli yang terlanjur terhapus. Akibatnya, crash pada fase 2 meninggalkan data duplikat, crash antara fase 2 dan 3 menyisakan data di kedua tempat, dan crash pada fase 3 menyebabkan kehilangan data secara permanen karena data asal dihapus sebelum dipastikan sukses tersalin seluruhnya. Firestore tidak mendukung transaksi ACID lintas koleksi untuk jumlah dokumen sebesar ini (limitasi transaksi Firestore adalah 500 dokumen).

### Keputusan
Mengubah mekanisme migrasi dengan pola dokumen kerja yang dapat dilanjutkan kembali (*resumable job document*) menggunakan metode *copy-then-soft-cutover*, dan tidak pernah melakukan penghapusan data asal dalam transaksi pemindahan yang sama.

Langkah-langkah yang diimplementasikan:
- Menyimpan log status migrasi pada koleksi `migration_jobs/{jobId}` yang mencatat: `userId`, `accountId`, `sharedAccountId`, status fase (`CREATED|COPYING|COPY_DONE|CUTOVER_DONE|CLEANUP_DONE`), daftar subcollection yang sukses disalin (`copiedCollections` berupa map string ke boolean), serta timestamp `createdAt`/`updatedAt`.
- Fase 1 tetap berjalan secara atomik karena memodifikasi kurang dari 10 dokumen (jauh di bawah batas transaksi Firestore).
- Fase 2 menyalin dokumen per subcollection dan menandai status suksesnya di dokumen kerja. Jika terjadi kegagalan sistem dan job dijalankan ulang, proses akan melompati subcollection yang sudah berhasil disalin sepenuhnya.
- Fase 3 hanya melakukan peralihan (*cutover*): Mengubah status job menjadi `CUTOVER_DONE` dan menandai akun privat lama dengan atribut `migrated: true`. Pada titik ini, data pada akun privat tetap dibiarkan utuh dan dapat dibaca jika diperlukan sebagai cadangan.
- Fase 4 adalah pembersihan data (*cleanup*): Proses penghapusan data pada subcollection asal dilakukan melalui proses latar belakang terpisah (*background cron*) atau endpoint admin khusus beberapa hari setelah status migrasi dipastikan sukses (`CUTOVER_DONE`).

### Konsekuensi
- **Kelebihan**: Risiko terburuk bergeser dari kehilangan data (*data loss*) menjadi data ganda yang tidak berbahaya (*duplicate data*). Jika proses terputus, sistem dapat mendeteksi kondisi tersebut melalui dokumen status kerja dan melanjutkannya secara aman. Operasi menjadi sepenuhnya bersifat *idempotent*.
- **Kekurangan**: Membutuhkan kapasitas penyimpanan Firestore tambahan selama masa tenggang pembersihan data, serta menambah kompleksitas berupa mesin status migrasi (*migration state machine*) di dalam backend.

---

## ADR-008: Urutan cutover — web callables dulu, Telegram bot terakhir

### Status
Accepted

### Konteks
Aplikasi frontend DompetCerdas memanggil 10 endpoint Firebase Functions menggunakan fungsi client `httpsCallable`. Proses migrasi ke backend Go berbasis REST API mengharuskan perubahan protokol ke panggilan `fetch` standar dengan menyertakan header otentikasi `Authorization: Bearer <FirebaseIDToken>`. Di sisi lain, webhook bot Telegram hanya dapat diarahkan ke satu alamat URL aktif pada satu waktu, membuat proses migrasi bot bersifat mutlak (*all-or-nothing*).

### Keputusan
Menyusun strategi transisi menjadi tiga tahapan berurutan:
1. Memindahkan fungsionalitas web callable secara bertahap, endpoint demi endpoint.
2. Memindahkan scheduler/cron jobs.
3. Memindahkan webhook bot Telegram pada tahap akhir sebagai satu kesatuan sakelar kendali (*hard switch*).

### Konsekuensi
- **Kelebihan**: Meminimalkan risiko kegagalan sistem secara masif. Integrasi web API yang lebih sederhana dapat diuji coba terlebih dahulu untuk memvalidasi stabilitas backend Go di produksi. Jika terjadi kendala pada web API, proses rollback dapat dilakukan per endpoint secara aman melalui pembaruan kode frontend. Proses rollback bot Telegram dapat dilakukan dengan cepat melalui satu perintah panggilan API `setWebhook` kembali ke URL Firebase Functions lama, didukung dengan mekanisme penguncian idempotensi `telegram_processed_updates` untuk mencegah pemrosesan ulang pesan selama masa transisi.
- **Kekurangan**: Masa coexistence (kedua backend hidup bersamaan di produksi) menjadi lebih panjang, menuntut sinkronisasi perubahan database yang ketat selama masa transisi tersebut.

---

## ADR-009: Firestore leader lock untuk cron selama coexistence

### Status
Accepted

### Konteks
Selama periode transisi migrasi backend (coexistence), kedua backend (Firebase Cloud Functions lama dan Go self-hosted baru) akan aktif secara bersamaan. Kedua sistem memiliki penjadwal tugas harian yang sama, salah satunya adalah pengiriman pengingat harian (*daily reminder*) ke Telegram pengguna. Jika tidak dibatasi, pengguna akan menerima pesan pengingat ganda setiap jamnya.

### Keputusan
Menggunakan mekanisme kunci kepemimpinan (*leader lock*) berbasis dokumen Firestore pada path `cron_locks/hourly_reminder` yang berisi field `lockedBy` (identitas instansi server) dan `lockedAt` (waktu penguncian). Sebelum menjalankan tugas cron, masing-masing backend harus mencoba melakukan penulisan kondisional (*conditional write*) untuk mengklaim kunci tersebut. Kunci yang berusia lebih dari 10 menit dianggap kedaluwarsa (*stale*) dan dapat diklaim ulang oleh instansi lain. Hanya backend yang memegang kunci aktif yang diperbolehkan mengeksekusi tugas cron tersebut.

### Konsekuensi
- **Kelebihan**: Menjamin pengguna tidak menerima pesan pengingat ganda tanpa perlu mematikan scheduler di salah satu server secara terburu-buru.
- **Kekurangan**: Menambah overhead berupa operasi baca/tulis ke database Firestore sebanyak satu kali per jam untuk setiap backend. Jika server pemegang kunci mengalami crash di tengah eksekusi tugas, eksekusi berikutnya baru dapat berjalan setelah masa tenggang 10 menit terlewati. Batasan ini dapat diterima untuk kebutuhan pengiriman pengingat. Mekanisme ini dapat dipertahankan atau dihapus setelah Firebase Cloud Functions lama sepenuhnya dinonaktifkan.

---

## ADR-010: Path resolution Firestore tetap 3 varian

### Status
Accepted

### Konteks
Pada backend lama, khususnya dalam `accountService.ts`, struktur jalur (*path*) dokumen Firestore diselesaikan secara dinamis ke dalam tiga variasi format tergantung dari tipe data akun yang diakses:
- Format legacy (akun lama sebelum multi-account): `users/{uid}/transactions`, `users/{uid}/categories`, `users/{uid}/simulations`
- Format private: `users/{uid}/accounts/{aid}/{transactions|categories|plans|budgets|debts|simulations|routine_expenses|routine_expense_records}`
- Format shared (akun bersama): `sharedAccounts/{sid}/{transactions|categories|plans|budgets|debts|simulations|routine_expenses|routine_expense_records}`

Perlu dicatat adanya inkonsistensi nama koleksi pada skema legacy: koleksi rencana anggaran dinamakan `simulations`, sedangkan pada skema private/shared dinamakan `plans`.

### Keputusan
Mempertahankan struktur pembacaan ketiga varian jalur Firestore tersebut di backend Go. Tidak melakukan migrasi pembersihan data lama (legacy data cleanup) sebagai bagian dari proyek rewrite ini.

### Konsekuensi
- **Kelebihan**: Data produksi pengguna lama tetap dapat diakses secara transparan tanpa risiko kegagalan proses migrasi skema database skala besar. Lingkup risiko pengerjaan rewrite tidak melebar ke pembersihan database.
- **Kekurangan**: Struktur penentuan jalur dokumen di dalam backend Go menjadi kompleks dan berisiko tinggi memicu celah keamanan akses data (*access-control bug*), misalnya kesalahan penulisan path yang mengakibatkan transaksi pengguna tertulis ke akun pengguna lain.
- **Mitigasi**: Seluruh akses data harus dibungkus dalam objek bertipe `AccountContext` dengan metode resolusi path yang eksplisit untuk setiap jenis subcollection. Tidak diperbolehkan melakukan pembuatan string path secara manual (*ad-hoc string building*) di luar modul akses data tersebut. Modul ini wajib ditunjang dengan unit testing yang mencakup pengujian ketiga variasi skema jalur.

---

## ADR-011: Auto-save gate dipertahankan + wajib audit logging

### Status
Accepted

### Konteks
Fitur `shouldAutoSaveText` (didefinisikan pada `bot/index.ts` baris 240-251) mengizinkan sistem mencatat transaksi keuangan ke database secara langsung tanpa memerlukan konfirmasi interaktif dari pengguna Telegram apabila memenuhi tiga syarat mutlak:
1. Hasil ekstraksi parser hanya menghasilkan tepat 1 item transaksi (*exactly 1 parsed item*).
2. Sumber analisis pemrosesan teks berasal dari jalur pencocokan pola ekspresi reguler lokal (*local regex path*), bukan dari fallback LLM.
3. Kategori transaksi berhasil diselesaikan secara langsung melalui nama kategori utama atau alias kategori (*direct/alias match*), tanpa bantuan klasifikasi LLM.

Kesalahan deteksi (*false positive*) pada fitur ini akan mengakibatkan kesalahan pencatatan laporan keuangan pengguna secara senyap, yang merupakan tingkat keparahan bug tertinggi pada aplikasi personal finance. Menghilangkan fitur ini sepenuhnya akan menurunkan kenyamanan pengguna (*UX degradation*) yang terbiasa mencatat transaksi tunggal secara instan.

### Keputusan
Mempertahankan seluruh kriteria seleksi filter `shouldAutoSaveText` secara identik di backend Go, dan mewajibkan penerapan log audit terstruktur (*structured audit logging*) untuk setiap transaksi yang lolos dari gerbang otomatisasi ini dengan format log:
`autoSaveTriggered=true`, disertai field data pendukung `amount`, `description`, `categoryName`, `telegramId`, dan `sourceText`.

### Konsekuensi
- **Kelebihan**: Kenyamanan pengguna bot Telegram tetap terjaga seperti sistem lama. Jika terjadi kesalahan pencatatan transaksi otomatis di produksi, tim pengembang dapat mendeteksi, menelusuri, dan menganalisis polanya secara cepat melalui log audit terstruktur tersebut.
- **Kekurangan**: Memerlukan jaminan konsistensi perilaku parser regex di Go agar berperilaku 100% identik dengan mesin regex TypeScript sebelumnya.
- **Persyaratan Validasi**: Tim wajib menggunakan pengujian *golden-fixture* yang didefinisikan pada dokumen Parity Contract (`docs/PARITY_CONTRACT.md` Fase 2) untuk membuktikan bahwa pola input yang sama menghasilkan keputusan auto-save yang sama di kedua bahasa pemrograman.

---

## ADR-012: Bug `last_month` diperbaiki, bukan direplikasi

### Status
Accepted (perbedaan perilaku yang disengaja / *intentional behavior divergence*)

### Konteks
Pada pembantu penanggalan (*date helper*) lama, kalkulasi rentang tanggal untuk kategori input "bulan lalu" memiliki bug bawaan saat dieksekusi di akhir bulan. Kode lama menghitung bulan sebelumnya menggunakan logika JavaScript berikut:
```ts
// Logika lama di JS/TS
setMonth(getMonth() - 1);
setDate(1);
```
Ketika kode ini dijalankan pada tanggal 31, terjadi luapan tanggal (*date overflow*). Sebagai ilustrasi, jika dijalankan pada tanggal 31 Maret:
1. `setMonth(1)` (Februari) dijalankan pada tanggal 31 menghasilkan tanggal 31 Februari. Karena Februari hanya memiliki 28/29 hari, tanggal otomatis bergeser maju (*overflow*) ke tanggal 3 Maret.
2. `setDate(1)` kemudian mengubah tanggal menjadi 1 Maret.
3. Kalkulasi tanggal akhir bulan lalu yang menggunakan metode `setDate(0)` pada objek tanggal tersebut menghasilkan tanggal 28 Februari.

Hasil akhir kalkulasi untuk rentang "bulan lalu" pada kasus di atas adalah: tanggal mulai `2026-03-01` dan tanggal selesai `2026-02-28`. Ini merupakan rentang terbalik (*inverted range*) yang tidak valid, sehingga pengguna yang melakukan kueri laporan dengan kata kunci "bulan lalu" pada tanggal 29, 30, atau 31 setelah bulan yang lebih pendek akan mendapatkan hasil kosong (0 transaksi).

### Keputusan
Memperbaiki bug kalkulasi tanggal tersebut di backend Go menggunakan operasi aritmatika kalender yang presisi, serta mengganti manipulasi offset zona waktu mentah UTC+7 (`getJakartaDate()`) dengan pemanggilan lokasi zona waktu resmi Go `time.LoadLocation("Asia/Jakarta")`.

Logika perbaikan di Go:
```go
firstOfThisMonth := time.Date(y, m, 1, 0, 0, 0, 0, loc)
start := firstOfThisMonth.AddDate(0, -1, 0)
end   := firstOfThisMonth.AddDate(0, 0, -1)
```

### Konsekuensi
- **Kelebihan**: Aplikasi mengembalikan data laporan keuangan yang benar dan valid bagi pengguna yang mengakses fitur di akhir bulan. Posisinya dilindungi oleh pengujian regresi (*regression test*) yang memastikan input tanggal pengujian `2026-03-31` wajib menghasilkan tanggal mulai `2026-02-01` dan tanggal selesai `2026-02-28` (terimplementasi pada modul `internal/shared/datetime/jakarta.go`).
- **Kekurangan**: Terjadi perbedaan perilaku respon data antara backend lama dan baru selama masa transisi. Untuk meminimalkan kebingungan pengguna, direkomendasikan untuk melakukan patch perbaikan bug ini pada kode TypeScript lama di produksi sesegera mungkin sebelum migrasi penuh rampung.

---

## ADR-013: Pergeseran satu hari `this_month` dan `custom_month` diperbaiki

### Status
Accepted (perbedaan perilaku yang disengaja / *intentional behavior divergence*)

### Konteks
Ditemukan saat menyusun fixture paritas Fase 2, bukan saat membaca kode. Fixture `date_range_legacy.json` merekam bahwa `this_month` dengan tanggal acuan 31 Maret 2026 menghasilkan rentang `2026-02-28` sampai `2026-03-30`, padahal seharusnya `2026-03-01` sampai `2026-03-31`.

Penyebabnya adalah pencampuran zona waktu. Kode lama membangun objek tanggal dengan konstruktor waktu lokal:
```ts
const s = new Date(now.getFullYear(), now.getMonth(), 1);
const e = new Date(now.getFullYear(), now.getMonth() + 1, 0);
```
lalu memformatnya lewat `toISOString().split('T')[0]`, yang mengonversi ke UTC. Karena Jakarta berada di UTC+7, tengah malam waktu lokal menjadi pukul 17:00 UTC pada hari sebelumnya, sehingga setiap batas tanggal bergeser mundur satu hari.

Dampaknya: setiap kueri "bulan ini" kehilangan transaksi pada tanggal terakhir bulan tersebut, dan malah menyertakan transaksi tanggal terakhir bulan sebelumnya. Hal yang sama terjadi pada `custom_month`.

Perhatikan bahwa `today`, `yesterday`, `this_week`, `last_week`, dan `days_ago` tidak terkena masalah ini, karena semuanya diturunkan dari `getJakartaDate()` yang sudah menambahkan offset 7 jam sebelum pemformatan.

### Keputusan
Menghitung batas bulan sepenuhnya di dalam `time.Location` Asia/Jakarta, tanpa perantara UTC:
```go
first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
last := first.AddDate(0, 1, -1)
```

### Konsekuensi
- **Kelebihan**: Kueri "bulan ini" dan bulan spesifik akhirnya mencakup seluruh hari pada bulan tersebut. Perbedaan ini ditegaskan sebagai perbedaan yang disengaja di `internal/shared/datetime/jakarta_legacy_test.go`, yang secara eksplisit melewati perbandingan nilai lama untuk kedua spec ini dan menyertakan komentar alasannya agar tidak "diperbaiki" kembali oleh pembaca berikutnya.
- **Kekurangan**: Total laporan bulanan akan berbeda antara backend lama dan baru selama masa koeksistensi. Untuk transaksi yang jatuh pada tanggal terakhir bulan, angka backend Go lebih tinggi. Seperti ADR-012, disarankan mem-patch kode TypeScript produksi lebih dulu agar selisih ini tidak muncul saat cutover.

---

## ADR-014: Wart pencocokan substring `QUERY_KEYWORDS` dipertahankan

### Status
Accepted (wart yang dipertahankan, membutuhkan keputusan produk)

### Konteks
Ditemukan saat menjalankan kode lama untuk membuat fixture. `containsQueryKeywords` di `transactionParsingService.ts:40-43` mencocokkan dengan `lower.includes(keyword)`, yaitu pencocokan substring, bukan batas kata. Karena `'list'` termasuk dalam daftar `QUERY_KEYWORDS`, transaksi yang sah ikut tertolak:

```
"bayar listrik 350rb"     -> DITOLAK  ('list' di dalam 'listrik')
"beli listrik pln 100rb"  -> DITOLAK
"listrik 350rb"           -> DITOLAK
"daftar belanja 50rb"     -> DITOLAK  ('daftar')
"riwayat 10rb"            -> DITOLAK  ('riwayat')
"total 5rb"               -> DITOLAK  ('total')
"bayar air 50rb"          -> diterima (tidak ada substring kata kunci)
```

Pengguna yang mengirim `bayar listrik 350rb` ke bot tidak mendapat transaksi tersimpan, dan juga tidak mendapat pesan error apa pun. Ini adalah perilaku produksi saat ini, bukan bug yang diperkenalkan oleh port Go.

Terdapat jalan keluar: kata kerja input eksplisit (`tambah`, `catat`, `input`, `masukin`, `masukan`) melewati pemeriksaan ini, sehingga `catat 350rb bayar listrik` tetap diterima.

### Alternatif yang Dipertimbangkan
1. **Memperbaikinya di Go** dengan pencocokan batas kata (`\blist\b`). Ditolak untuk saat ini: ini mengubah pesan apa saja yang diterima bot, sehingga merupakan keputusan produk, bukan keputusan porting. Memperbaikinya diam-diam berarti port Go berperilaku berbeda dari produksi tanpa catatan.
2. **Mempertahankan apa adanya tanpa pengujian.** Ditolak: wart yang tidak diuji akan "diperbaiki" oleh kontributor berikutnya tanpa sadar mengubah perilaku produksi.

### Keputusan
Mempertahankan pencocokan substring persis seperti aslinya (`strings.Contains`), dan mem-*pin*-nya lewat pengujian `TestQueryKeywordSubstringWart` di `internal/modules/transaction/parser_test.go` sehingga perilakunya terlihat dan disengaja.

### Konsekuensi
- **Kelebihan**: Paritas perilaku terjaga. Wart terdokumentasi dan terlindungi pengujian, bukan tersembunyi.
- **Kekurangan**: Pengguna tetap tidak bisa mencatat tagihan listrik dengan frasa alami. Perbaikannya menunggu keputusan produk. Bila disetujui, perbaikan wajib disertai pembaruan fixture dan ADR baru yang menggantikan ADR ini.

---

## ADR-015: `EscapeMarkdown` dipersempit ke 4 karakter spesial V1, bukan direplikasi dari MarkdownV2

### Status
Accepted (perbedaan perilaku yang disengaja / *intentional behavior divergence*)

### Konteks
`escapeMarkdown` di backend lama (`responseFormatter.ts:8-10`) meng-escape seluruh character class MarkdownV2 (`_*[]()~`>#+-=|{}.!\`), padahal setiap pemanggilan `sendMessage`/`editMessageText` — baik di legacy maupun di Go — memakai `parse_mode: "Markdown"` (V1). Telegram V1 hanya mengenali `_`, `*`, `` ` ``, `[` sebagai karakter yang bisa di-escape; backslash di depan karakter lain tidak dikonsumsi oleh parser dan tampil apa adanya ke pengguna.

Dampaknya terlihat langsung di produksi: deskripsi transaksi hasil AI seperti "Transaksi pembelian emas sebesar Rp150.000 pada tanggal 2 Agustus 2026." dirender sebagai "Transaksi pembelian emas sebesar Rp150\\.000 pada tanggal 2 Agustus 2026\\." — backslash mentah muncul di depan setiap titik. Ini terjadi di semua pesan yang melewati `EscapeMarkdown`: konfirmasi draft transaksi, daftar kategori, ringkasan saldo, dll.

`docs/PARITY_CONTRACT.md` bagian 4.4 sebelumnya secara eksplisit meminta perilaku over-escaping ini **dipertahankan apa adanya** karena "output byte-nya sudah menjadi bagian dari pesan produksi". Catatan itu ditulis saat merencanakan porting, sebelum bug ini teramati langsung dari pesan produksi nyata.

### Alternatif yang Dipertimbangkan
1. **Pindah ke `parse_mode: "MarkdownV2"`.** Ditolak: MarkdownV2 mewajibkan escaping ketat di seluruh teks statis (header, tombol, dll), bukan cuma field dinamis. Blast radius-nya besar — satu karakter statis yang lolos di-escape akan membuat Telegram menolak pesan (`can't parse entities`), dan seluruh pesan bot perlu diaudit ulang.
2. **Mempertahankan character class lama apa adanya.** Ditolak: ini bukan wart yang netral seperti pembulatan `999999 -> Rp 1000k` — ini bug tampilan yang nyata-nyata terlihat pengguna di setiap transaksi, dan tidak menyentuh nominal/penyimpanan sehingga aman diperbaiki tanpa risiko salah catat keuangan.

### Keputusan
Mempersempit `mdEscaper` di `internal/modules/telegram/formatter.go` menjadi hanya 4 karakter yang benar-benar spesial di V1 (`_`, `*`, `` ` ``, `[`) ditambah backslash literal itu sendiri (agar backslash pada input pengguna tidak menyatu dengan delimiter yang disisipkan tepat sesudahnya, misalnya penutup `*bold*`).

```go
var mdEscaper = regexp.MustCompile(`([_*\[` + "`" + `\\])`)
```

Fixture `testdata/parity/markdown_escape.json` dan seluruh fixture parity Telegram lain yang mengandung karakter yang terdampak (`telegram_transaction_details.json`, `telegram_category_list.json`, `telegram_draft_preview.json`, `telegram_auto_saved.json`) diperbarui untuk mengunci perilaku V1 yang benar, ditambah test baru `TestEscapeMarkdown_V1DoesNotOverEscape` yang menegaskan `.`, `!`, `-`, `(`, `)` tidak lagi di-escape sementara `_`, `*`, `` ` ``, `[` tetap di-escape.

### Konsekuensi
- **Kelebihan**: Pesan Telegram produksi tidak lagi menampilkan backslash mentah ke pengguna. Tidak ada risiko pada parsing/penyimpanan nominal karena perubahan ini murni di lapisan rendering teks.
- **Kekurangan**: Byte output Go tidak lagi identik dengan backend lama untuk domain ini — divergensi ini didokumentasikan di `docs/PARITY_CONTRACT.md` bagian 5 menggantikan catatan "wajib dipertahankan" di bagian 4.4.

---

## Keputusan yang Masih Terbuka (Open Questions)

Bagian ini mendokumentasikan beberapa keputusan arsitektur yang belum disepakati sepenuhnya dan membutuhkan keputusan lanjutan:

### 1. Keamanan URL Nota Belanja Publik (Public Receipt URLs)
- **Deskripsi**: Layanan `storageService.ts` saat ini memanggil fungsi `.makePublic()` setelah proses unggah gambar nota belanja berhasil dilakukan. Hal ini menyebabkan setiap berkas gambar nota belanja pengguna dapat diakses secara publik oleh siapa saja melalui alamat URL statis:
  `https://storage.googleapis.com/{bucket}/users/{userId}/attachments/receipt_{unixMillis}.jpg`
- **Permasalahan**: Pola alamat URL tersebut sangat mudah ditebak (*guessable path*) karena hanya mengombinasikan `userId` dan timestamp milidetik. Nota belanja sering kali memuat informasi sensitif seperti nama toko, rincian barang, harga belanjaan, hingga nama lengkap pengguna.
- **Kebutuhan Data/Keputusan**: Apakah backend Go harus beralih menggunakan URL bertanda tangan dengan batas waktu kedaluwarsa (*signed URLs with expiry*) untuk membatasi akses? Perubahan ini memerlukan penyesuaian di sisi aplikasi frontend React agar dapat menangani proses pembacaan ulang URL yang telah kedaluwarsa.

### 2. Skalabilitas Cron Pengingat Telegram (Reminder Cron Scalability)
- **Deskripsi**: Fungsi `processDailyNoTransactionReminders()` saat ini mengeksekusi kueri `collectionGroup('telegram_link').get()` tanpa disertai klausa penyaringan (*where clause*), lalu melakukan pemfilteran data secara manual di memori server untuk menghindari keharusan membuat indeks komposit (*composite index*) di Firestore.
- **Permasalahan**: Operasi ini melakukan pemindaian berskala O(N) terhadap seluruh dokumen tautan Telegram pengguna setiap jamnya. Seiring bertambahnya basis pengguna, operasi ini akan memicu pemborosan memori dan biaya kueri Firestore.
- **Kebutuhan Data/Keputusan**: Menentukan batas ambang jumlah pengguna aktif Telegram yang mengharuskan pembuatan indeks komposit resmi atau perubahan arsitektur ke sistem distribusi antrean pesan (*scheduled-fanout*).

### 3. Latensi Pembaruan Kategori (Category Cache Staleness)
- **Deskripsi**: Struktur penyimpanan cache kategori pengguna menggunakan `Map` lokal pada backend lama yang memiliki masa kedaluwarsa (TTL) 24 jam. Pada arsitektur Cloud Functions, cache ini sering kali terhapus secara otomatis akibat siklus pembersihan instansi server (*cold start*). Namun, pada backend Go yang hidup terus-menerus, data cache ini akan bertahan penuh selama 24 jam.
- **Permasalahan**: Jika pengguna mengubah nama kategori melalui aplikasi web, bot Telegram pengguna tersebut berpotensi menampilkan kategori lama selama maksimal 24 jam karena data cache yang belum diperbarui di memori backend Go.
- **Kebutuhan Data/Keputusan**: Menentukan apakah durasi TTL cache kategori harus diperkecil, atau mengimplementasikan sistem pembatalan cache berbasis kejadian (*event-driven cache invalidation*) menggunakan Firestore listener.

### 4. Pelonggaran Aturan Keamanan Firestore (Firestore Rules TODO)
- **Deskripsi**: Aturan keamanan Firestore (`firestore.rules`) untuk pembaruan transaksi bersama (*shared transaction update*) saat ini dikonfigurasi dengan toleransi longgar:
  `allow update: if canAccessSharedAccount(...)`
  Toleransi ini diterapkan disertai dengan komentar penjelas di atasnya: `// TODO: Re-enable after debugging`.
- **Permasalahan**: Walaupun backend Go akan menerapkan pemeriksaan otorisasi yang ketat di sisi server untuk setiap transaksi, pelonggaran aturan keamanan langsung di Firestore memicu celah bypass akses jika pengguna berinteraksi langsung dengan Firebase SDK di frontend.
- **Kebutuhan Data/Keputusan**: Memastikan apakah aturan pengetatan akses di `firestore.rules` harus diaktifkan kembali dan disinkronkan dengan spesifikasi otorisasi backend Go.

### 5. Konsolidasi Jalur Skema Legacy (Legacy Path Consolidation)
- **Deskripsi**: Dokumen ADR-010 memutuskan untuk mempertahankan dukungan terhadap 3 variasi skema jalur Firestore tanpa melakukan migrasi data lama ke skema baru.
- **Permasalahan**: Tim pengembang harus terus memelihara kode percabangan logika jalur data (*path routing logic branching*) dan skema database yang tidak konsisten di masa mendatang.
- **Kebutuhan Data/Keputusan**: Perlu ditentukan waktu pelaksanaan proyek terpisah di masa mendatang untuk memigrasi seluruh data pengguna legacy ke struktur data berbasis akun privat terstandar, sehingga kode pendukung legacy di backend Go dapat dihapus secara permanen.

---

## Format Pembuatan ADR Baru

Setiap pencatatan keputusan arsitektur baru di masa mendatang wajib mengikuti struktur format standar berikut untuk menjaga konsistensi dokumentasi:

```markdown
## ADR-XXXX: [Judul Keputusan]

### Status
[Proposed | Accepted | Superseded | Deprecated] (cantumkan referensi ADR lama jika berstatus Supersedes)

### Konteks
[Jelaskan latar belakang teknis atau bisnis, masalah yang dihadapi, batasan sistem, dan kendala yang memicu perlunya keputusan ini diambil]

### Alternatif yang Dipertimbangkan (Opsional)
- **Alternatif A**: [Deskripsi ringkas serta pro & kontra]
- **Alternatif B**: [Deskripsi ringkas serta pro & kontra]

### Keputusan
[Nyatakan keputusan akhir yang dipilih secara tegas dan jelas beserta pustaka/teknologi/pola desain yang disepakati]

### Konsekuensi
- **Kelebihan**: [Daftar dampak positif dan efisiensi yang didapatkan]
- **Kekurangan**: [Daftar kompromi, biaya tambahan, overhead, atau utang teknis yang harus diterima]
```

---

## Referensi

- `docs/MIGRATION_PLAN.md` — Rencana pelaksanaan migrasi fase demi fase.
- `docs/PARITY_CONTRACT.md` — Kontrak keselarasan perilaku sistem dan strategi verifikasi *golden-fixture*.
- Repositori lama: `/Users/mthidayat/Dev-Labs/dompet_cerdas` (akses baca untuk referensi).
- Draf rancangan lama: `docs/PRD_REWRITE_GO.md` pada repositori lama (menyusul keputusan penggunaan 9Router yang kemudian digantikan oleh [ADR-002](#adr-002-gemini-sdk-langsung-drop-9router)).
