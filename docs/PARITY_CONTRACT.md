# Parity Contract: Node.js to Go Backend Rewrite

## 1. Tujuan Dokumen
Dokumen ini menetapkan kontrak perilaku (behavioral contract) untuk penulisan ulang backend DompetCerdas dari Node.js (TypeScript) ke Go. DompetCerdas adalah aplikasi pencatatan keuangan pribadi berbasis Telegram. Pengguna dapat mengetik pesan teks bebas dalam bahasa Indonesia untuk mencatat transaksi keuangan mereka.

Aplikasi ini memiliki fitur auto-save. Fitur ini menyimpan transaksi secara otomatis tanpa konfirmasi pengguna jika tingkat kepercayaan (confidence) analisis tinggi. Jika terjadi perbedaan kecil pada regex atau pembulatan angka antara backend lama dan baru, backend Go dapat menuliskan nominal yang salah ke catatan keuangan pengguna. Kesalahan pencatatan ini adalah kegagalan paling kritis bagi aplikasi keuangan. Kontrak ini meminimalkan risiko tersebut dengan mengunci perilaku kode melalui pengujian fixture (golden fixture tests) yang ditangkap langsung dari eksekusi backend lama.

## 2. Prinsip Parity
Implementasi backend Go wajib mematuhi aturan berikut untuk menjamin kesamaan perilaku sistem:
1. **Fixture Diambil dari Eksekusi Nyata**: Seluruh data uji dalam dokumen ini diperoleh dengan menjalankan kode Node.js lama secara langsung. Jangan membuat perkiraan nilai keluaran hanya dengan membaca kode sumber.
2. **Dokumentasikan Setiap Divergensi**: Setiap perubahan perilaku yang sengaja dilakukan harus dicatat dalam dokumen keputusan arsitektur.
3. **Kunci Setiap Keunikan Perilaku (Warts)**: Perilaku aneh atau kutu (warts) yang ada pada sistem lama harus dipertahankan demi kompatibilitas. Tulis unit test khusus untuk mengunci perilaku tersebut di Go.
4. **Kegagalan Uji Fixture Memblokir Rilis**: Pengujian integrasi Go yang gagal mencocokkan fixture akan menghentikan proses build dan rilis.

## 3. Metodologi Golden Fixture
Validasi logika parsing dan formatting dilakukan dengan membandingkan input nyata terhadap output yang dihasilkan oleh kode Node.js lama. Proses verifikasi ini mengikuti alur kerja berikut:

### 3.1 Alur Kerja Pembuatan Fixture
1. Buat skrip pengekstraksi (harness script) di repositori lama (`/Users/mthidayat/Dev-Labs/dompet_cerdas`). Skrip ini mengimpor fungsi TypeScript yang sudah dikompilasi, kemudian mengeksekusi sekumpulan kasus uji.
2. Skrip tersebut mengekspor pasangan `{input, output}` ke berkas JSON.
3. Simpan berkas JSON tersebut di repositori baru (`/Users/mthidayat/Dev-Labs/dompet_cerdas_go`) pada folder `testdata/parity/`.
4. Unit test di Go akan memuat berkas JSON tersebut dan memverifikasi bahwa kode Go menghasilkan output yang identik byte-for-byte.

### 3.2 Skema JSON Fixture
Setiap berkas JSON di `testdata/parity/` harus mengikuti format struktur berikut:
```json
{
  "domain": "Amount Parsing",
  "fixtures": [
    {
      "input": "25k",
      "expected": 25000
    },
    {
      "input": "abc",
      "expected": null
    }
  ]
}
```

### 3.3 Contoh Loader Unit Test di Go
Berikut adalah rancangan fungsi pengujian di Go untuk membaca berkas fixture:
```go
package parity_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type ParityTestCase struct {
	Input    string      `json:"input"`
	Expected interface{} `json:"expected"`
}

type ParityFixture struct {
	Domain   string           `json:"domain"`
	Fixtures []ParityTestCase `json:"fixtures"`
}

func LoadParityFixture(t *testing.T, filename string) ParityFixture {
	t.Helper()
	path := filepath.Join("testdata", "parity", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read fixture file %s: %v", filename, err)
	}
	var fixture ParityFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("Failed to parse JSON fixture %s: %v", filename, err)
	}
	return fixture
}
```

## 4. Kontrak per Domain

### 4.1 Amount Parsing
Logika pemrosesan angka nominal berada pada file `transactionParsingService.ts`. Pencarian token nominal menggunakan regex berikut:
```
/(?:rp\s*|idr\s*)?\d[\d.,]*\s*(?:k|rb|ribu|jt|juta|m|milyar|miliar)?\b/gi
```

Kode TypeScript untuk menganalisis token tersebut adalah sebagai berikut:
```typescript
// transactionParsingService.ts (Line 114-136)
const cleaned = token.toLowerCase()
  .replace(/^rp\s*/i, '').replace(/^idr\s*/i, '').trim();
const suffixMatch = cleaned.match(/(k|rb|ribu|jt|juta|m|milyar|miliar)$/i);
const suffix = suffixMatch ? suffixMatch[0].toLowerCase() : '';
const numberPart = suffix ? cleaned.slice(0, cleaned.length - suffix.length).trim() : cleaned;
const baseValue = normalizeNumberString(numberPart, Boolean(suffix));
if (baseValue === null) return null;
let multiplier = 1;
if (['k','rb','ribu'].includes(suffix)) multiplier = 1000;
else if (['jt','juta'].includes(suffix)) multiplier = 1_000_000;
else if (['m','milyar','miliar'].includes(suffix)) multiplier = 1_000_000_000;
return Math.round(baseValue * multiplier);
```

Fungsi pembantu `normalizeNumberString` memiliki logika berikut:
```typescript
// transactionParsingService.ts (Line 100-112)
if (hasSuffix) {
  const normalized = raw.replace(/,/g, '.');
  const value = parseFloat(normalized);
  return Number.isFinite(value) ? value : null;
}
const digitsOnly = raw.replace(/[.,]/g, '');
const value = parseInt(digitsOnly, 10);
return Number.isFinite(value) ? value : null;
```

#### Verified Fixture Table untuk Amount Parsing
Tabel berikut berisi pasangan input dan output yang telah terverifikasi melalui eksekusi Node.js:

| input | expected |
| :--- | :--- |
| "25k" | 25000 |
| "25rb" | 25000 |
| "25ribu" | 25000 |
| "25K" | 25000 |
| "2,5ribu" | 2500 |
| "2jt" | 2000000 |
| "2juta" | 2000000 |
| "2,5jt" | 2500000 |
| "1.5jt" | 1500000 |
| "1m" | 1000000000 |
| "1milyar" | 1000000000 |
| "1miliar" | 1000000000 |
| "2,5miliar" | 2500000000 |
| "50000" | 50000 |
| "150.000" | 150000 |
| "20,000" | 20000 |
| "1.500.000" | 1500000 |
| "150.50" | 15050 |
| "Rp 25000" | 25000 |
| "rp25k" | 25000 |
| "IDR 50.000" | 50000 |
| "" | null |
| "jt" | null |
| "abc" | null |
| "500" | 500 |
| "999" | 999 |
| "1000" | 1000 |
| "999999" | 999999 |
| "1000000" | 1000000 |
| "25 rb" | 25000 |
| "2.5jt" | 2500000 |
| "10rb" | 10000 |
| "1,25juta" | 1250000 |
| "0,5jt" | 500000 |
| "rp 1.250.000" | 1250000 |
| "5m" | 5000000000 |

#### Penjelasan Keunikan (Quirks)
Ada dua perilaku spesifik yang wajib dipertahankan:
* **The `150.50` wart**: Ketika input tidak memiliki suffix akhiran, tanda titik dianggap sebagai pemisah ribuan. Hal ini menyebabkan string `150.50` dibersihkan dari titik dan koma, menghasilkan nilai `15050`, bukan `150.5`.
* **Koma sebagai Desimal Hanya dengan Suffix**: Karakter koma dianggap sebagai desimal jika token diakhiri suffix seperti `jt` atau `ribu`. Sebagai contoh, `2,5jt` dibaca sebagai desimal dan menghasilkan `2500000`. Sebaliknya, `20,000` tanpa suffix akan dibersihkan tanda komanya dan dibaca sebagai pemisah ribuan biasa, menghasilkan `20000`.

---

### 4.2 Currency Formatting
Pemberian format mata uang rupiah dilakukan oleh dua fungsi dalam `responseFormatter.ts` (Line 284-299):
```typescript
function formatRupiah(amount) {
  if (amount >= 1_000_000) return `Rp ${(amount/1_000_000).toFixed(1)}jt`;
  else if (amount >= 1_000) return `Rp ${(amount/1_000).toFixed(0)}k`;
  else return `Rp ${amount.toLocaleString('id-ID')}`;
}
function formatExactRupiah(amount) { return `Rp ${amount.toLocaleString('id-ID')}`; }
```

#### Verified Fixture Table untuk Currency Formatting
Tabel di bawah merinci hasil pemformatan untuk berbagai kondisi nilai:

| amount | formatRupiah | formatExactRupiah |
| :--- | :--- | :--- |
| 500 | "Rp 500" | "Rp 500" |
| 999 | "Rp 999" | "Rp 999" |
| 1000 | "Rp 1k" | "Rp 1.000" |
| 25000 | "Rp 25k" | "Rp 25.000" |
| 25600 | "Rp 26k" | "Rp 25.600" |
| 999999 | "Rp 1000k" | "Rp 999.999" |
| 1000000 | "Rp 1.0jt" | "Rp 1.000.000" |
| 1049999 | "Rp 1.0jt" | "Rp 1.049.999" |
| 2500000 | "Rp 2.5jt" | "Rp 2.500.000" |
| 0 | "Rp 0" | "Rp 0" |
| -50000 | "Rp -50.000" | "Rp -50.000" |
| -150000 | "Rp -150.000" | "Rp -150.000" |
| -2500000 | "Rp -2.500.000" | "Rp -2.500.000" |

#### Penjelasan Keunikan (Quirks)
Perhatikan detail berikut agar pemformatan di Go tidak melenceng:
* **Fungsi `toFixed` Melakukan Pembulatan**: Angka dibulatkan ke nilai terdekat saat dikonversi ke string. Kasus `25600` dibulatkan ke atas menjadi `Rp 26k`. Input `999999` dibulatkan ke atas menjadi `Rp 1000k`, bukan dipotong ke bawah menjadi `Rp 999k`.
* **Nilai Negatif Mengalir ke Cabang Terakhir (Fall-Through)**: Logika pembanding menguji nilai bertanda (signed value). Kondisi `-50000 >= 1000` bernilai salah, sehingga nilai negatif langsung dilempar ke cabang `toLocaleString`. Hal ini menghasilkan output `Rp -50.000`, bukan format ringkas seperti `-Rp 50k`.
* **Posisi Tanda Minus**: Karakter negatif diletakkan setelah teks `"Rp "`. Format yang dihasilkan adalah `"Rp -50.000"` karena interpolasi string menaruh output `toLocaleString` di belakang prefix.

---

### 4.3 Date Range Resolution
Resolusi rentang tanggal digunakan untuk memproses perintah pencarian laporan transaksi pada berkas `queryService.ts` dan `utils/date.ts`.
Zona waktu bawaan sistem lama menggunakan offset statis 7 jam untuk mensimulasikan waktu Jakarta:
```typescript
export function getJakartaDate(date = new Date()) {
  const offset = 7 * 60 * 60 * 1000;
  return new Date(date.getTime() + offset);
}
```

Format string tanggal diambil dari hasil pemisahan ISO string: `d.toISOString().split('T')[0]`.

#### Spesifikasi Rentang Tanggal (Date Range Specifications)
Resolusi rentang tanggal mendukung beberapa pilihan parameter berikut:

| spec | start date | end date | keterangan |
| :--- | :--- | :--- | :--- |
| `today` | today | today | Hari berjalan |
| `yesterday` | today - 1 | today - 1 | Satu hari sebelum hari berjalan |
| `this_week` | Monday | today | Hari Senin minggu ini sampai hari berjalan. Perhitungan offset Senin: `dayOfWeek = getDay()` (Minggu=0); `mondayOffset = (dayOfWeek === 0) ? -6 : 1 - dayOfWeek` |
| `last_week` | today - 6 | today | **Salah Kaprah (Misnomer)**: Menghasilkan rentang 7 hari terakhir termasuk hari berjalan, bukan minggu kalender sebelumnya. |
| `this_month` | First day of current month | Last day of current month | Tanggal 1 sampai tanggal terakhir di bulan berjalan. |
| `last_month` | First day of previous month | Last day of previous month | **Mengandung Bug di Node.js**: Menggunakan `setMonth(getMonth()-1)` lalu `setDate(1)`. Jika dieksekusi pada tanggal 31 Maret, `setMonth(1)` mengubah tanggal menjadi 31 Februari, yang meluap otomatis menjadi 3 Maret. Eksekusi `setDate(1)` berikutnya mengubahnya menjadi 1 Maret. Akhir bulan dihitung lewat `setDate(0)` yang menunjuk ke 28 Februari. Hasil rentang menjadi terbalik: start `2026-03-01`, end `2026-02-28`. |
| `custom_month` (`YYYY-MM`) | First day of YYYY-MM | Last day of YYYY-MM | Tanggal 1 sampai tanggal terakhir pada bulan terpilih. |
| `days_ago:N` | today - N | today - N | Satu hari spesifik pada N hari lalu (start == end). Ini bukan rentang sepanjang N hari. |

#### Kasus Uji Regresi yang Diperbaiki di Go
Untuk memperbaiki bug `last_month` pada tanggal 31 Maret, backend Go harus menggunakan fungsi manipulasi tanggal bawaan Go yang aman. Kasus uji regresi berikut wajib lolos pada Go:
* **Tanggal Referensi**: `2026-03-31`
* **Hasil yang Diharapkan**: start `2026-02-01`, end `2026-02-28`

Implementasi pemotongan tanggal yang benar di Go:
```go
firstOfThisMonth := time.Date(y, m, 1, 0, 0, 0, 0, loc)
start := firstOfThisMonth.AddDate(0, -1, 0)
end := firstOfThisMonth.AddDate(0, 0, -1)
```

---

### 4.4 Markdown Escaping

Produksi memakai **Telegram Markdown V1**, bukan MarkdownV2. Seluruh pemanggilan `sendMessage`/`editMessageText` di `bot/index.ts`, `reminderService.ts`, `bot/commands/*.ts`, dan `index.ts` memakai `{ parse_mode: 'Markdown' }` — tidak ada satu pun `MarkdownV2`. Jangan mengganti ke V2 saat porting: V2 punya aturan escaping yang lebih ketat dan akan mengubah rendering pesan yang sudah berjalan.

Fungsi escaping di `responseFormatter.ts:8-10`:

```javascript
export function escapeMarkdown(text: string | number | null | undefined): string {
    return String(text ?? '').replace(/([_*\[\]()~`>#+\-=|{}.!\\])/g, '\\$1');
}
```

Perhatikan dua hal yang mudah terlewat:

Character class-nya sebenarnya adalah daftar karakter khusus MarkdownV2, tetapi dipakai bersama `parse_mode: 'Markdown'` (V1). Ini berlebihan untuk V1 dan menyebabkan backslash bocor mentah ke pesan user setiap kali deskripsi transaksi mengandung titik, tanda seru, tanda kurung, dsb. **Backend Go sengaja tidak mereplikasi perilaku ini** — lihat divergensi di bagian 5 dan implementasi aktual di `internal/modules/telegram/formatter.go`. Karakter yang di-escape di Go hanya `_*\`[` plus backslash literal, bukan seluruh daftar di bawah.

Input non-string ditangani lewat `String(text ?? '')`, sehingga `null` dan `undefined` menjadi string kosong, dan angka dikonversi lebih dulu. Port Go harus menyediakan perilaku setara untuk nilai kosong.

#### Catatan Implementasi di Go (RE2)

Backtick tidak bisa ditulis di dalam raw string literal Go, jadi perlu dikonkatenasi.

Replacement `\\$1` di JS setara `\$1` di Go, tetapi hati-hati: Go menafsirkan `$1` di dalam raw string sebagai grup capture, sehingga penulisannya harus lewat interpreted string atau `${1}` bila ada digit yang mengikutinya.

Implementasi aktual **tidak** mereplikasi character class JS di atas (lihat divergensi bagian 5) — hanya 4 karakter yang benar-benar spesial di V1 plus backslash literal:

```go
var mdEscaper = regexp.MustCompile(`([_*\[` + "`" + `\\])`)

func EscapeMarkdown(text string) string {
	return mdEscaper.ReplaceAllString(text, `\$1`)
}
```

Fixture domain ini (`testdata/parity/markdown_escape.json`) sudah diperbarui untuk mengunci perilaku V1 yang benar, bukan lagi replay byte-for-byte dari `escapeMarkdown` legacy — lihat catatan di dalam fixture tersebut.

---

### 4.5 NLU Intent Classification
Klasifikasi niat pengguna (intent classification) ditangani oleh berkas `nluService.ts` yang berukuran 952 baris.
Daftar tipe intent yang valid adalah sebagai berikut:
`query_expenses`, `query_income`, `query_balance`, `add_transaction`, `category_breakdown`, `query_details`, `list_categories`, `financial_advice`, `savings_strategy`, `expense_analysis`, `unknown`.

#### Regex Utama NLU
Berikut adalah kumpulan pola regex lokal yang dipakai untuk mendeteksi intent sebelum melempar ke fallback AI:

```javascript
TRANSACTION_QUERY_PATTERN = /trans(?:aksi)?s?|transs|transaski|transsaksi|tranaksi|transactions?|txs?/i
DETAIL_QUERY_PATTERN      = /detail|rincian|apa\s+aja|apa\s+saja|list|tampilkan|lihat|show|tunjukkan/i
RANKING_QUERY_PATTERN     = /\b(top|last|latest|terakhir|tertinggi|terbesar|terbanyak|highest|biggest|largest)\b/i
LIMIT_QUERY_PATTERN       = /\b(top|last|latest)\s+\d+\b|(?:^|\s)\d+\s+(?:trans(?:aksi)?s?|transs|transaski|transsaksi|tranaksi|transactions?|txs?|item|data|pengeluaran)\b/i
ENTRY_PREFIX_REGEX        = /^(tambah(?:in)?|catat(?:kan)?|input|masukin|masukan|record|log)\s+/i
BALANCE_PATTERN           = /^(berapa\s+)?(saldo|balance|sisa\s+uang)(\s+(sekarang|saya|kamu|aku|gw))?(\s+berapa)?$/i
```

#### Alasan Toleransi Typo (Typo-Tolerance Rationale)
Variasi typo untuk kata transaksi (`transaski`, `transsaksi`, `tranaksi`, `transs`, `txs`) ditambahkan pada changelog v2.8.10. Kebutuhan ini muncul karena sistem sebelumnya sering salah mengenali query laporan (misalnya "show 10 last transs") sebagai instruksi untuk menyimpan transaksi baru. Toleransi ini harus dipertahankan penuh di porting Go untuk menghindari false positive pada parser transaksi.

#### Logika Penentuan Alur NLU (Routing Logic)
Fungsi `shouldPreferAIIntentParsing` memisahkan proses klasifikasi antara regex lokal dengan NLU berbasis Gemini LLM. Query yang sederhana (canonical queries) diproses secara lokal untuk menghemat biaya token API. Jika query mengandung unsur pengurutan (ranking), batasan limit, atau memiliki pola pengetikan yang kompleks, sistem akan menggunakan Gemini LLM.

---

### 4.6 Transaction Parsing Pipeline
Alur kerja pemrosesan pesan transaksi baru di `transactionParsingService.ts` berjalan melalui beberapa tahap penguraian:

#### 1. Pembagian Pesan Multi-Transaksi (Multi-Transaction Splitting)
Jika pengguna mengirimkan beberapa transaksi sekaligus dalam satu pesan, teks dipecah berdasarkan urutan prioritas pembatas berikut:
1. Karakter baris baru `\n` jika ditemukan dalam teks.
2. Karakter titik koma `;` jika ditemukan dalam teks.
3. Karakter koma `,` dengan syarat jumlah pencocokan token nominal (`AMOUNT_REGEX`) dalam pesan lebih dari 1.
4. Kata sambung berupa regex `/\s+(?:dan|lalu|terus|trus|&)\s+/i` dengan syarat jumlah pencocokan token nominal dalam pesan lebih dari 1.

#### 2. Ekstraksi Segmen Transaksi Lokal
Setiap potongan segmen hasil pemisahan akan dianalisis secara mandiri:
* Deteksi petunjuk kategori lewat kata kunci (misal "kategori Food" menghasilkan petunjuk kategori `Food`).
* Cari kecocokan token nominal. Segmen harus memiliki **tepat satu** nominal rupiah. Jika segmen memiliki lebih dari satu nominal atau tidak ada sama sekali, segmen tersebut langsung dilempar ke parser fallback berbasis Gemini LLM (maksimal 20 segmen).
* Hapus token nominal dari teks segmen.
* Pembersihan sisa tanda baca di akhir teks menggunakan regex `/[\s:;\-–—]+$/`.
* Pastikan teks deskripsi tidak kosong setelah dibersihkan. Jika kosong, buang draf transaksi tersebut melalui fungsi `normalizeParsedTransactionDrafts`.

#### 3. Pintu Gerbang Auto-Save (Auto-Save Gate)
Proses penyimpanan otomatis langsung ke database Firestore tanpa konfirmasi pengguna adalah alur yang sangat sensitif. Auto-save hanya boleh berjalan jika semua kondisi di bawah terpenuhi:
* Pesan hanya berisi tepat 1 draf transaksi.
* Proses ekstraksi berhasil diselesaikan melalui parser regex lokal (bukan lewat fallback Gemini LLM), **atau** lewat ekstraksi foto struk Gemini Vision dengan `confidenceScore` numerik melebihi secara ketat ambang `ReceiptAutoSaveConfidenceThreshold` (90) — lihat ADR-016.
* Kategori transaksi berhasil dideteksi tanpa bantuan LLM (cocok langsung dengan nama kategori asli atau daftar alias kategori).

Jalur teks bebas (fallback Gemini LLM) dan voice note tidak pernah mengisi `confidenceScore`, sehingga keduanya selalu melewati draft konfirmasi. Jika salah satu kondisi tidak terpenuhi, backend wajib membuat session draf transaksi di Firestore dan membalas dengan tombol konfirmasi di Telegram. Backend Go harus menuliskan log audit terstruktur (`autoSaveTriggered`, nominal, deskripsi, kategori, serta `usedAI` dan `confidenceScore` untuk jalur struk) saat auto-save berhasil dijalankan agar kesalahan sistem di produksi dapat dianalisis dengan mudah.

Validasi awal ini langsung menolak teks pesan yang terindikasi sebagai query laporan keuangan agar tidak masuk ke pipeline ekstraksi transaksi.

---

## 5. Divergensi yang Disengaja
Beberapa perubahan perilaku dari sistem lama sengaja dirancang pada backend Go untuk memperbaiki kesalahan operasional:

| Area | Old Behavior | New Behavior | Justifikasi | Test yang Memverifikasi |
| :--- | :--- | :--- | :--- | :--- |
| `last_month` resolution | Mengalami rollover salah ke bulan Maret ketika dihitung pada tanggal 31 Maret (menghasilkan range terbalik start `2026-03-01`, end `2026-02-28`). | Menggunakan kalkulasi `jakarta.go` yang memotong tanggal secara aman dan mengembalikan range awal-akhir bulan yang presisi. | Perbaikan bug kalkulasi keuangan bulanan. | `internal/shared/datetime/jakarta_test.go` |
| Timezone approach | Menggunakan offset statis (+7 jam dalam milidetik) untuk memanipulasi objek Date Javascript yang sensitif zona waktu lokal server. | Menggunakan pustaka standar Go `time.LoadLocation("Asia/Jakarta")` dan tipe data `time.Time` ter-zona. | Menghilangkan bug perbedaan jam pencatatan akibat drift zona waktu server. | `internal/shared/datetime/jakarta_test.go` |
| `EscapeMarkdown` character class | Meng-escape seluruh karakter khusus MarkdownV2 (`_*[]()~\`>#+-=\|{}.!\\`) walau parse_mode produksi tetap `Markdown` (V1). V1 tidak mengenali escape untuk karakter selain `_*\`[`, sehingga backslash-nya bocor mentah ke pesan user (mis. deskripsi transaksi tampil sebagai `"Rp150\.000"`, bukan `"Rp150.000"`). | Hanya meng-escape 4 karakter yang benar-benar spesial di V1 (`_`, `*`, `` ` ``, `[`) ditambah backslash literal itu sendiri (agar tidak menyatu dengan delimiter yang disisipkan tepat sesudahnya, mis. penutup `*bold*`). | Bug tampilan murni (tidak menyentuh parsing/penyimpanan nominal), muncul di setiap pesan konfirmasi transaksi produksi, dan webhook Telegram sudah full cutover ke Go — bagian 4.4 yang meminta "dipertahankan apa adanya" ditulis sebelum bug ini teramati langsung dari screenshot produksi. | `internal/modules/telegram/formatter_test.go` (`TestEscapeMarkdown_Fixtures`, `TestEscapeMarkdown_V1DoesNotOverEscape`) |
| Auto-save foto struk | Setiap unggahan foto struk selalu melewati draft konfirmasi karena hasil ekstraksi Gemini Vision ditandai sebagai hasil AI dan gerbang auto-save menolak seluruh hasil AI. | Foto struk dengan `confidenceScore` numerik melebihi secara ketat ambang 90 dan kategori yang terselesaikan deterministik langsung auto-save tanpa konfirmasi (ADR-016). Jalur teks bebas dan voice tidak berubah — tetap selalu konfirmasi. | Menghilangkan langkah konfirmasi yang tidak perlu untuk struk yang terbaca jelas, dengan tetap memproteksi dari salah baca model lewat ambang tinggi + kategori deterministik + audit log. | `internal/modules/transaction/parser_test.go` (`TestShouldAutoSave`), `internal/modules/telegram/draft_test.go` (`TestAutoSaveGate`) |
| Lampiran foto struk | Bot legacy mengunggah foto struk ke Firebase Storage dengan `makePublic()` dan menyimpan URL publik statis di field `attachment` transaksi; URL itu mudah ditebak sehingga bukti bayar pengguna dapat diakses siapa pun. | Objek disimpan **privat** ke path account-scoped (`users/{uid}/accounts/{accountId}/attachments/...` atau `sharedAccounts/{id}/attachments/...`) yang dilindungi `storage.rules`; transaksi menyimpan `attachment.path` dengan `url` kosong, dan web app me-resolve URL tampilan dari path via `getDownloadURL` (ADR-017). Kegagalan unggah tetap non-fatal. | Menutup celah keamanan URL publik tanpa mengorbankan tampilan lampiran; path sesuai aturan akses Storage yang sudah ada. | `internal/modules/telegram/receipt_upload_test.go`, `internal/modules/transaction/write_test.go` (`TestBuildManualPayload_WritesAttachment`) |

*Catatan: Perubahan perilaku di luar kontrak ini (seperti penghitungan token token-usage Gemini API dan transisi OCR Tesseract ke Gemini Vision) dicatat secara terpisah pada dokumen `docs/DECISIONS.md`.*

---

## 6. Wart yang Dipertahankan
Beberapa keanehan atau kutu (warts) dari kode lama sengaja dipertahankan agar tidak mengubah perilaku konsumsi pengguna yang sudah terbiasa:

| Wart | Contoh | Alasan Dipertahankan | Test yang Mem-pin |
| :--- | :--- | :--- | :--- |
| `150.50` dot-stripping | Input `"150.50"` di-parse menjadi nominal `15050` | Mencegah kerusakan perilaku parsing pada data input lama yang mengandalkan titik sebagai pemisah ribuan tanpa suffix. | `internal/shared/money/rupiah_test.go` |
| `999999` compact format | Nilai `999999` diformat menjadi `"Rp 1000k"` | Fungsi `toFixed(0)` bawaan JavaScript membulatkan angka ke atas sebelum memformat nominal ribuan. | `internal/shared/money/rupiah_test.go` |
| Negatives compact omission | Nilai `-50000` diformat menjadi `"Rp -50.000"` (bukan `"Rp -50k"`) | Nilai negatif langsung jatuh ke cabang else pada fungsi `formatRupiah` karena pembandingan bernilai salah. | `internal/shared/money/rupiah_test.go` |
| `last_week` misnomer | Range `last_week` menghasilkan 7 hari terakhir ke belakang dari hari berjalan. | Menghindari kebingungan pengguna jika data grafik di bot mendadak berubah rentang harinya. | `internal/shared/datetime/jakarta_test.go` |
| `days_ago:N` single-day span | Parameter `days_ago:2` menghasilkan rentang mulai hari dan akhir hari yang sama pada 2 hari lalu. | Digunakan untuk mencari laporan pengeluaran khusus pada satu tanggal spesifik di masa lalu. | `internal/shared/datetime/jakarta_test.go` |

---

## 7. Perbedaan Regex JS → Go (RE2)
Berikut adalah daftar perbedaan mesin regular expression antara JavaScript dan Go (RE2) yang harus diperhatikan saat menulis kode porting:
* **Tidak Ada Lookahead dan Lookbehind**: RE2 tidak mendukung assertion lookaround seperti `(?=...)` atau `(?<=...)`. Jika regex lama mengandung pola ini, Anda harus menulis ulang alur parsing menggunakan logika kode Go secara prosedural.
* **Stateless vs Stateful**: Regex di JavaScript yang menggunakan bendera global (`/g`) menyimpan index pencarian terakhir (`lastIndex`) pada objek regex tersebut. Mesin regex Go bersifat stateless. Hapus baris pembersihan `lastIndex = 0` dan gunakan fungsi pencarian kelompok seperti `FindAllString` jika ingin mengambil semua kecocokan.
* **Batasan Word Boundary (`\b`)**: Batasan kata `\b` pada RE2 hanya mendeteksi batas karakter ASCII `[a-zA-Z0-9_]`. Periksa perilaku pencarian kata jika teks transaksi mengandung karakter non-ASCII atau emoji di ujung kata kunci.
* **Bendera Case-Insensitive**: Gunakan penanda `(?i)` di awal pola regex Go untuk menggantikan bendera `/i` pada JavaScript. Contoh: `/transaksi/i` menjadi `(?i)transaksi`.

---

## 8. Checklist Verifikasi per Fase
Gunakan checklist ini sebelum menyatakan porting satu domain selesai:
* [ ] **Fixture Generated**: Skrip pengekstrak di Node.js lama telah dijalankan dan menghasilkan berkas fixture JSON.
* [ ] **Fixture Committed**: Berkas JSON fixture telah disimpan di folder `testdata/parity/` repositori baru.
* [ ] **Go Test Loads Fixture**: Unit test Go berhasil membaca data dari berkas JSON tersebut.
* [ ] **All Cases Pass**: Seluruh kasus uji pada berkas fixture menghasilkan nilai yang identik di Go.
* [ ] **Warts Pinned**: Kasus uji khusus untuk mengunci wart (misal nominal `150.50`) telah ditambahkan dan lulus uji.
* [ ] **Divergences Documented**: Perbedaan perilaku yang disetujui telah terdata di bab 5 dokumen ini.

---

## 9. Referensi
* **Amount Parsing Code**: `functions/src/services/transactionParsingService.ts` (Line 49, 100-136)
* **Currency Formatting Code**: `functions/src/services/responseFormatter.ts` (Line 284-299)
* **Telegram Escaping Code**: `functions/src/services/responseFormatter.ts` (Line 400-420)
* **Date Resolution Code**: `functions/src/utils/date.ts` (Line 1-24) dan `functions/src/services/queryService.ts`
* **Old Node Test Files**:
  * `functions/test-nlu.js`
  * `functions/test-transaction-parser.js`
  * `functions/test-intent-routing.js`
* **Sibling Documentation**:
  * Rencana Migrasi: `docs/MIGRATION_PLAN.md`
  * Dokumen Keputusan Arsitektur: `docs/DECISIONS.md`
  
---
🤖 Generated with [opencode](https://opencode.ai/)