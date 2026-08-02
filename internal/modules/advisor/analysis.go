package advisor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

// SystemInstruction is the advisor's guardrail prompt, ported verbatim from
// generateFinancialInsightsWithUsage (geminiService.ts:225).
//
// It is what keeps the model inside personal-finance analysis; loosening it
// re-opens the off-topic answers the legacy isOffTopicResponse check exists to
// catch.
const SystemInstruction = `Kamu adalah AI Financial Advisor untuk DompetCerdas, aplikasi manajemen keuangan personal.

ATURAN KETAT (WAJIB DIIKUTI):
1. Kamu HANYA boleh menganalisis data transaksi yang diberikan user
2. JANGAN jawab pertanyaan tentang:
   - Investasi saham/crypto/reksadana/trading
   - Berita ekonomi/politik/sosial
   - Topik di luar manajemen keuangan personal user
   - Hal-hal tidak berhubungan dengan data transaksi yang diberikan
   - Pertanyaan umum tentang finansial yang tidak spesifik ke data user

3. Jika user tanya off-topic atau data tidak cukup, jawab:
   "Maaf, saya hanya bisa menganalisis data transaksi keuangan yang tersedia. Ketik /help untuk panduan."

4. Format output:
   - Bahasa Indonesia casual tapi profesional
   - Gunakan emoji untuk readability (📊 💡 💰 🎯 ✅ ⚠️)
   - Max 500 kata per response
   - Fokus pada actionable insights, bukan general advice
   - Berikan estimasi penghematan konkret dalam Rupiah
   - Sebutkan kategori & nominal spesifik dari DATA

5. Struktur jawaban standar:
   📊 Summary (ringkas 1-2 kalimat)
   💡 Key Insights (2-4 poin berdasarkan data)
   💰 Rekomendasi (3-5 action items konkret)
   🎯 Quick Wins (1-2 tips termudah)

6. JANGAN:
   - Membuat asumsi di luar data yang diberikan
   - Memberikan saran investasi
   - Membahas topik politik/agama/sosial
   - Menggunakan bahasa formal kaku
   - Memberikan saran umum seperti "sebaiknya menabung" tanpa data spesifik

7. DO:
   - Analisa pola spending dari data
   - Identifikasi outlier & anomali
   - Berikan rekomendasi spesifik dengan estimasi saving
   - Referensi kategori & transaksi konkret
   - Gunakan bahasa yang encouraging tapi honest`

// offTopicMarkers are the refusal phrases that mean the model ignored the
// guardrail, ported from isOffTopicResponse (geminiService.ts:328).
var offTopicMarkers = []string{
	"tidak bisa membantu",
	"di luar kemampuan saya",
	"sebagai ai",
	"as an ai",
	"i'm sorry, but",
}

// IsOffTopic reports whether a generated answer drifted off-topic and should be
// replaced with the standard refusal rather than shown.
func IsOffTopic(response string) bool {
	lower := strings.ToLower(response)
	for _, marker := range offTopicMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// OffTopicReply is what the user sees when the model wandered.
const OffTopicReply = "Maaf, saya hanya bisa menganalisis data transaksi keuangan yang tersedia. Ketik /help untuk panduan."

// AnalysisTransaction is the sanitised transaction shape sent to the model. Only
// these five fields leave the system; ids and creator metadata never do.
type AnalysisTransaction struct {
	Amount       int64
	Description  string
	CategoryName string
	Date         string
}

// selection sizes, ported from selectRelevantTransactions (advisorService.ts:155).
const (
	selectTopByAmount = 30
	selectMostRecent  = 20
)

// SelectRelevantTransactions samples a token-efficient subset: the largest
// amounts, the most recent, and one per category.
//
// The three strategies overlap on purpose — outliers, current trend, and breadth
// each surface something the others miss — and duplicates are dropped in the
// order the strategies are listed, so a big recent transaction is kept once.
func SelectRelevantTransactions(details []transaction.Detail, maxCount int) []AnalysisTransaction {
	expenses := make([]transaction.Detail, 0, len(details))
	for _, d := range details {
		if d.Type == domain.TransactionTypeExpense {
			expenses = append(expenses, d)
		}
	}
	if len(expenses) == 0 || maxCount <= 0 {
		return nil
	}

	byAmount := append([]transaction.Detail(nil), expenses...)
	sort.SliceStable(byAmount, func(i, j int) bool { return byAmount[i].Amount > byAmount[j].Amount })
	byAmount = byAmount[:min(len(byAmount), min(selectTopByAmount, maxCount))]

	byDate := append([]transaction.Detail(nil), expenses...)
	sort.SliceStable(byDate, func(i, j int) bool { return byDate[i].Date > byDate[j].Date })
	byDate = byDate[:min(len(byDate), min(selectMostRecent, maxCount))]

	var diverse []transaction.Detail
	seenCategory := make(map[string]bool)
	for _, d := range expenses {
		if !seenCategory[d.Category] {
			seenCategory[d.Category] = true
			diverse = append(diverse, d)
		}
	}

	seenID := make(map[string]bool)
	selected := make([]AnalysisTransaction, 0, maxCount)
	for _, group := range [][]transaction.Detail{byAmount, byDate, diverse} {
		for _, d := range group {
			if seenID[d.ID] || len(selected) >= maxCount {
				continue
			}
			seenID[d.ID] = true

			description := d.Description
			if description == "" {
				description = "No description"
			}
			categoryName := d.Category
			if categoryName == "" {
				categoryName = "Uncategorized"
			}

			selected = append(selected, AnalysisTransaction{
				Amount:       d.Amount,
				Description:  description,
				CategoryName: categoryName,
				Date:         d.Date,
			})
		}
	}

	return selected
}

// Thresholds for pattern detection, ported from detectSpendingPatterns
// (advisorService.ts:204).
const (
	smallTransactionAmount   = 50_000
	smallTransactionMinCount = 15
	outlierMultiplier        = 2
	concentrationMaxCategory = 3
	concentrationMinTx       = 20
	dailyHabitMinDays        = 25
)

// DetectSpendingPatterns turns the sampled transactions into short observations
// the prompt can lean on, so the model is not left to infer them.
func DetectSpendingPatterns(txs []AnalysisTransaction) []string {
	if len(txs) == 0 {
		return nil
	}

	var patterns []string

	small := 0
	var total int64
	dates := make(map[string]bool)
	categories := make(map[string]bool)
	for _, t := range txs {
		if t.Amount < smallTransactionAmount {
			small++
		}
		total += t.Amount
		dates[t.Date] = true
		categories[t.CategoryName] = true
	}

	if small > smallTransactionMinCount {
		patterns = append(patterns, fmt.Sprintf("Banyak transaksi kecil: %d transaksi < Rp 50rb", small))
	}

	average := float64(total) / float64(len(txs))
	large := 0
	for _, t := range txs {
		if float64(t.Amount) > average*outlierMultiplier {
			large++
		}
	}
	if large > 0 {
		patterns = append(patterns, fmt.Sprintf("%d transaksi besar (>2x rata-rata) terdeteksi", large))
	}

	if len(categories) <= concentrationMaxCategory && len(txs) > concentrationMinTx {
		patterns = append(patterns, fmt.Sprintf("Spending terkonsentrasi di %d kategori saja", len(categories)))
	}

	if len(dates) > dailyHabitMinDays {
		patterns = append(patterns, "Transaksi hampir setiap hari")
	}

	return patterns
}

// FormatCompactAmount renders an amount the way the prompts do: "1.2jt", "450rb",
// or the bare number. This is prompt-internal and deliberately terser than the
// user-facing money formatter, to keep the token count down.
func FormatCompactAmount(amount int64) string {
	switch {
	case amount >= 1_000_000:
		return fmt.Sprintf("%.1fjt", float64(amount)/1_000_000)
	case amount >= 1_000:
		return fmt.Sprintf("%.0frb", float64(amount)/1_000)
	default:
		return fmt.Sprintf("%d", amount)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
