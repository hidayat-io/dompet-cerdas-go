package advisor

import (
	"fmt"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

// Mode selects which analysis to run. The values match the legacy callable's
// accepted modes and the Telegram intents that map onto them.
type Mode string

const (
	// ModeHealth is the general overview ("gimana keuanganku?").
	ModeHealth Mode = "HEALTH"
	// ModeSavings is the forward-looking savings plan ("tips hemat").
	ModeSavings Mode = "SAVINGS"
	// ModeSpending is the cut-without-suffering analysis.
	ModeSpending Mode = "SPENDING"
)

// PromptInput is everything the prompt builders read. Assembling it is the
// caller's job, which keeps prompt construction pure and testable.
type PromptInput struct {
	PeriodLabel      string
	ExpenseTotal     int64
	ExpenseCount     int
	LastMonthTotal   int64
	Balance          int64
	Breakdown        []transaction.CategoryBreakdown
	Transactions     []AnalysisTransaction
	Patterns         []string
	CategoryStats    []CategoryStats
	SelectedForCount int
}

// CategoryStats summarises one category's spread, used by the spending analysis
// to spot categories with cheaper alternatives.
type CategoryStats struct {
	Category string
	Count    int
	Total    int64
	Average  int64
	Max      int64
	Min      int64
}

// BuildCategoryStats computes per-category spread from the raw details.
func BuildCategoryStats(breakdown []transaction.CategoryBreakdown, details []transaction.Detail) []CategoryStats {
	stats := make([]CategoryStats, 0, len(breakdown))

	for _, cat := range breakdown {
		var total, max, min int64
		count := 0
		for _, d := range details {
			if d.Category != cat.Category {
				continue
			}
			if count == 0 || d.Amount > max {
				max = d.Amount
			}
			if count == 0 || d.Amount < min {
				min = d.Amount
			}
			total += d.Amount
			count++
		}

		var average int64
		if count > 0 {
			average = total / int64(count)
		}

		stats = append(stats, CategoryStats{
			Category: cat.Category,
			Count:    cat.Count,
			Total:    cat.Amount,
			Average:  average,
			Max:      max,
			Min:      min,
		})
	}

	return stats
}

// topCategories returns at most n breakdown rows, which arrive already sorted by
// amount.
func topCategories(breakdown []transaction.CategoryBreakdown, n int) []transaction.CategoryBreakdown {
	if len(breakdown) > n {
		return breakdown[:n]
	}
	return breakdown
}

// trendPercent is the month-over-month change. With no prior month there is no
// trend to report, and dividing would be undefined, so it reads as flat — the
// legacy behavior.
func trendPercent(current, previous int64) float64 {
	if previous <= 0 {
		return 0
	}
	return float64(current-previous) / float64(previous) * 100
}

func trendArrow(trend float64) string {
	if trend > 0 {
		return "↑ naik"
	}
	return "↓ turun"
}

// BuildPrompt renders the data prompt for a mode, porting the three templates in
// advisorService.ts (analyzeFinancialHealth, generateSavingsStrategy,
// analyzeExpenseReduction).
func BuildPrompt(mode Mode, in PromptInput) string {
	switch mode {
	case ModeSavings:
		return buildSavingsPrompt(in)
	case ModeSpending:
		return buildSpendingPrompt(in)
	default:
		return buildHealthPrompt(in)
	}
}

func buildHealthPrompt(in PromptInput) string {
	trend := trendPercent(in.ExpenseTotal, in.LastMonthTotal)

	var categoryLines []string
	for _, c := range topCategories(in.Breakdown, 5) {
		categoryLines = append(categoryLines, fmt.Sprintf("- %s: %.0f%% (Rp %s, %d transaksi)",
			c.Category, c.Percentage, FormatCompactAmount(c.Amount), c.Count))
	}

	var txLines []string
	for i, t := range in.Transactions {
		txLines = append(txLines, fmt.Sprintf("%d. Rp %s - %s [%s] (%s)",
			i+1, FormatCompactAmount(t.Amount), t.Description, t.CategoryName, t.Date))
	}

	patterns := "- Pola spending normal"
	if len(in.Patterns) > 0 {
		patterns = "- " + strings.Join(in.Patterns, "\n- ")
	}

	return fmt.Sprintf(`ANALISA KEUANGAN USER (%s):

📊 CURRENT PERIOD:
Total pengeluaran: Rp %s (%d transaksi)
Trend vs bulan lalu: %.1f%% %s
Saldo saat ini: Rp %s

BREAKDOWN KATEGORI:
%s

TOP TRANSAKSI TERBESAR:
%s

POLA SPENDING:
%s

TUGAS:
1. Analisa kesehatan keuangan user secara keseluruhan
2. Identifikasi pola spending yang baik/buruk
3. Berikan 3-5 insight spesifik berdasarkan DATA DI ATAS
4. Rekomendasi konkret & actionable (bukan general advice)
5. Estimasi potensi penghematan jika ada

FORMAT JAWABAN (gunakan emoji untuk readability):
📊 Summary (2-3 kalimat ringkas)

💡 Key Insights (3-4 poin penting)

💰 Rekomendasi (3-5 action items spesifik)

🎯 Quick Win (1-2 tips termudah untuk immediate action)

ATURAN:
- Bahasa Indonesia casual tapi profesional
- Fokus pada DATA yang diberikan, jangan membuat asumsi
- Sebutkan nominal & kategori spesifik
- Max 400 kata
- NO general advice (contoh buruk: "sebaiknya menabung", "kurangi pengeluaran")
- YES specific advice (contoh baik: "Kurangi 3x makan di warteg mahal (Rp 450rb) → switch ke warteg biasa = save Rp 900rb/bulan")`,
		in.PeriodLabel,
		FormatCompactAmount(in.ExpenseTotal), in.ExpenseCount,
		trend, trendArrow(trend),
		FormatCompactAmount(in.Balance),
		strings.Join(categoryLines, "\n"),
		strings.Join(txLines, "\n"),
		patterns)
}

func buildSavingsPrompt(in PromptInput) string {
	var categoryLines []string
	for _, c := range topCategories(in.Breakdown, 5) {
		categoryLines = append(categoryLines, fmt.Sprintf("- %s: Rp %s (%.0f%%)",
			c.Category, FormatCompactAmount(c.Amount), c.Percentage))
	}

	limit := len(in.Transactions)
	if limit > 30 {
		limit = 30
	}
	var txLines []string
	for i, t := range in.Transactions[:limit] {
		txLines = append(txLines, fmt.Sprintf("%d. Rp %s - %s [%s]",
			i+1, FormatCompactAmount(t.Amount), t.Description, t.CategoryName))
	}

	return fmt.Sprintf(`STRATEGI HEMAT UNTUK USER (Target: Bulan Depan):

📊 DATA %s:
Total pengeluaran: Rp %s

BREAKDOWN KATEGORI:
%s

SAMPLE TRANSAKSI BESAR:
%s

TUGAS:
Buat strategi hemat untuk bulan depan yang:
1. REALISTIS - bisa diterapkan tanpa mengurangi quality of life
2. KONKRET - dengan target nominal saving per kategori
3. PRIORITAS - dari yang termudah ke tersulit
4. ACTIONABLE - user bisa langsung execute

FORMAT:
🎯 Target Hemat Bulan Depan: [nominal total]

💰 Strategi Hemat (by priority):

1️⃣ [Kategori]: [Action konkret]
   Current: Rp [X]
   Target: Rp [Y]
   Saving: Rp [X-Y]
   Cara: [step-by-step]

2️⃣ [dst...]

📝 Tips Eksekusi:
- [tips praktis 1]
- [tips praktis 2]

ATURAN:
- Target saving total 15-25%% dari pengeluaran current
- Prioritas kategori discretionary (Food, Shopping, Entertainment)
- Jangan touch kategori essential (Bill, Healthcare)
- Bahasa Indonesia, max 350 kata`,
		strings.ToUpper(in.PeriodLabel),
		FormatCompactAmount(in.ExpenseTotal),
		strings.Join(categoryLines, "\n"),
		strings.Join(txLines, "\n"))
}

func buildSpendingPrompt(in PromptInput) string {
	var txLines []string
	for i, t := range in.Transactions {
		txLines = append(txLines, fmt.Sprintf("%d. Rp %s - %s [%s] (%s)",
			i+1, FormatCompactAmount(t.Amount), t.Description, t.CategoryName, t.Date))
	}

	var statLines []string
	for _, g := range in.CategoryStats {
		statLines = append(statLines, fmt.Sprintf("- %s: %d transaksi, total Rp %s, avg Rp %s, range Rp %s-%s",
			g.Category, g.Count, FormatCompactAmount(g.Total), FormatCompactAmount(g.Average),
			FormatCompactAmount(g.Min), FormatCompactAmount(g.Max)))
	}

	return fmt.Sprintf(`ANALISA PENGELUARAN YANG BISA DIKURANGI (Tanpa Suffering):

📋 TRANSAKSI %s (%d items):
%s

📊 BREAKDOWN PER KATEGORI:
%s

TUGAS:
Identifikasi 5-7 area spending yang BISA DIKURANGI tanpa mengurangi quality of life:

KRITERIA:
✅ Frekuensi tinggi + nilai besar (habit expensive)
✅ Kategori discretionary (Food, Shopping, Entertainment)
✅ Variasi harga tinggi (ada alternatif lebih murah)
✅ Bukan essential (bukan Bill, Healthcare, commute transport)

FORMAT:
🔍 Temuan Pengeluaran Bisa Dihemat:

1️⃣ [Kategori/Item Spesifik]
   📌 Terdeteksi: [frekuensi & pattern]
   💰 Alternative: [solusi lebih murah]
   💵 Potensi Saving: Rp [X]/bulan
   😊 Impact: [tingkat kesulitan: mudah/medium/sulit]

2️⃣ [dst...]

📊 TOTAL POTENSI HEMAT: Rp [X]/bulan

🎯 LOW-HANGING FRUITS (prioritas tertinggi):
- [item termudah 1]
- [item termudah 2]

ATURAN:
- Fokus pada DATA yang terlihat (jangan asumsi)
- Sebutkan nominal & frekuensi spesifik
- Beri alternatif konkret, bukan saran umum
- Sort by potential saving (terbesar dulu)
- Bahasa Indonesia, max 400 kata`,
		strings.ToUpper(in.PeriodLabel),
		len(in.Transactions),
		strings.Join(txLines, "\n"),
		strings.Join(statLines, "\n"))
}
