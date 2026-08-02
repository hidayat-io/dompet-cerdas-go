package advisor

import (
	"strings"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

func detail(id string, amount int64, category, date string, t domain.TransactionType) transaction.Detail {
	return transaction.Detail{
		ID: id, Amount: amount, Category: category, Date: date, Type: t,
		Description: "tx " + id, CategoryResolved: true,
	}
}

func TestSelectRelevantTransactions_ExcludesIncome(t *testing.T) {
	details := []transaction.Detail{
		detail("a", 100_000, "Makanan", "2026-01-10", domain.TransactionTypeExpense),
		detail("b", 5_000_000, "Gaji", "2026-01-01", domain.TransactionTypeIncome),
	}

	got := SelectRelevantTransactions(details, 50)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 — income must be excluded", len(got))
	}
	if got[0].CategoryName != "Makanan" {
		t.Errorf("category = %q, want Makanan", got[0].CategoryName)
	}
}

func TestSelectRelevantTransactions_Deduplicates(t *testing.T) {
	// The same row is both the largest and the most recent; it must appear once.
	details := []transaction.Detail{
		detail("big", 900_000, "Makanan", "2026-01-31", domain.TransactionTypeExpense),
		detail("small", 10_000, "Transport", "2026-01-02", domain.TransactionTypeExpense),
	}

	got := SelectRelevantTransactions(details, 50)

	seen := map[string]int{}
	for _, tx := range got {
		seen[tx.Description]++
	}
	for desc, count := range seen {
		if count > 1 {
			t.Errorf("%q appears %d times, want 1", desc, count)
		}
	}
}

func TestSelectRelevantTransactions_RespectsMaxCount(t *testing.T) {
	var details []transaction.Detail
	for i := 0; i < 200; i++ {
		details = append(details, detail(string(rune('a'+i%26))+string(rune('0'+i/26)),
			int64(i+1)*1000, "Kategori", "2026-01-01", domain.TransactionTypeExpense))
	}

	if got := SelectRelevantTransactions(details, 10); len(got) > 10 {
		t.Errorf("len = %d, want at most 10", len(got))
	}
}

func TestSelectRelevantTransactions_EmptyInputs(t *testing.T) {
	if got := SelectRelevantTransactions(nil, 50); got != nil {
		t.Errorf("nil input should give nil, got %v", got)
	}
	details := []transaction.Detail{detail("a", 1000, "X", "2026-01-01", domain.TransactionTypeExpense)}
	if got := SelectRelevantTransactions(details, 0); got != nil {
		t.Errorf("maxCount 0 should give nil, got %v", got)
	}
}

func TestDetectSpendingPatterns(t *testing.T) {
	// 21 rows: concentration needs strictly more than 20, so 20 would miss it.
	var many []AnalysisTransaction
	for i := 0; i < 21; i++ {
		many = append(many, AnalysisTransaction{Amount: 10_000, CategoryName: "Makanan", Date: "2026-01-01"})
	}

	patterns := DetectSpendingPatterns(many)

	joined := strings.Join(patterns, "|")
	if !strings.Contains(joined, "Banyak transaksi kecil") {
		t.Errorf("expected a small-transaction pattern, got %v", patterns)
	}
	if !strings.Contains(joined, "terkonsentrasi") {
		t.Errorf("expected a concentration pattern, got %v", patterns)
	}
}

// The concentration threshold is exclusive. Pinning the boundary keeps a
// refactor from turning "> 20" into ">= 20" unnoticed.
func TestDetectSpendingPatterns_ConcentrationBoundary(t *testing.T) {
	build := func(n int) []AnalysisTransaction {
		var txs []AnalysisTransaction
		for i := 0; i < n; i++ {
			txs = append(txs, AnalysisTransaction{Amount: 100_000, CategoryName: "Makanan", Date: "2026-01-01"})
		}
		return txs
	}

	if strings.Contains(strings.Join(DetectSpendingPatterns(build(20)), "|"), "terkonsentrasi") {
		t.Error("20 transactions must not trigger the concentration pattern")
	}
	if !strings.Contains(strings.Join(DetectSpendingPatterns(build(21)), "|"), "terkonsentrasi") {
		t.Error("21 transactions should trigger the concentration pattern")
	}
}

func TestDetectSpendingPatterns_EmptyGivesNothing(t *testing.T) {
	if got := DetectSpendingPatterns(nil); len(got) != 0 {
		t.Errorf("patterns = %v, want none", got)
	}
}

func TestFormatCompactAmount(t *testing.T) {
	tests := []struct {
		amount int64
		want   string
	}{
		{amount: 500, want: "500"},
		{amount: 25_000, want: "25rb"},
		{amount: 1_500_000, want: "1.5jt"},
		{amount: 0, want: "0"},
	}

	for _, tt := range tests {
		if got := FormatCompactAmount(tt.amount); got != tt.want {
			t.Errorf("FormatCompactAmount(%d) = %q, want %q", tt.amount, got, tt.want)
		}
	}
}

func TestIsOffTopic(t *testing.T) {
	if !IsOffTopic("Maaf, sebagai AI saya tidak bisa membantu soal itu.") {
		t.Error("a refusal should be detected as off-topic")
	}
	if IsOffTopic("📊 Summary: pengeluaranmu turun 12% bulan ini.") {
		t.Error("a normal analysis must not be flagged")
	}
}

func TestModeFromString(t *testing.T) {
	for _, valid := range []string{"HEALTH", "SAVINGS", "SPENDING"} {
		if _, err := ModeFromString(valid); err != nil {
			t.Errorf("ModeFromString(%q) errored: %v", valid, err)
		}
	}
	if _, err := ModeFromString("INVESTASI"); err == nil {
		t.Error("an unknown mode must be rejected")
	}
}

// The guardrail prompt is what keeps the advisor inside personal finance; the
// test pins its key clauses so they cannot be trimmed by accident.
func TestSystemInstruction_KeepsGuardrails(t *testing.T) {
	for _, clause := range []string{
		"HANYA boleh menganalisis data transaksi",
		"Investasi saham/crypto/reksadana/trading",
		"JANGAN",
	} {
		if !strings.Contains(SystemInstruction, clause) {
			t.Errorf("system instruction lost the clause %q", clause)
		}
	}
}

func TestBuildPrompt_IncludesTheData(t *testing.T) {
	in := PromptInput{
		PeriodLabel:  "bulan ini",
		ExpenseTotal: 1_500_000,
		ExpenseCount: 12,
		Breakdown: []transaction.CategoryBreakdown{
			{Category: "Makanan", Amount: 900_000, Percentage: 60, Count: 8},
		},
		Transactions: []AnalysisTransaction{
			{Amount: 250_000, Description: "Nasi padang", CategoryName: "Makanan", Date: "2026-01-10"},
		},
	}

	for _, mode := range []Mode{ModeHealth, ModeSavings, ModeSpending} {
		got := BuildPrompt(mode, in)
		if !strings.Contains(got, "Makanan") {
			t.Errorf("%s prompt is missing the category data", mode)
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s prompt has a broken format verb:\n%s", mode, got)
		}
	}
}

func TestBuildCategoryStats(t *testing.T) {
	breakdown := []transaction.CategoryBreakdown{{Category: "Makanan", Amount: 300_000, Count: 3}}
	details := []transaction.Detail{
		detail("a", 50_000, "Makanan", "2026-01-01", domain.TransactionTypeExpense),
		detail("b", 100_000, "Makanan", "2026-01-02", domain.TransactionTypeExpense),
		detail("c", 150_000, "Makanan", "2026-01-03", domain.TransactionTypeExpense),
		detail("d", 900_000, "Transport", "2026-01-04", domain.TransactionTypeExpense),
	}

	got := BuildCategoryStats(breakdown, details)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Min != 50_000 || got[0].Max != 150_000 || got[0].Average != 100_000 {
		t.Errorf("stats = %+v, want min 50000 / max 150000 / avg 100000", got[0])
	}
}

// With no prior month there is nothing to compare against; the trend must read
// as flat rather than dividing by zero.
func TestTrendPercent_NoPreviousMonth(t *testing.T) {
	if got := trendPercent(500_000, 0); got != 0 {
		t.Errorf("trendPercent = %v, want 0", got)
	}
}
