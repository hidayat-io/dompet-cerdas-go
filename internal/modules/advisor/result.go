package advisor

import (
	"strconv"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// AnalysisResult is the payload the web advisor screen renders. The field names
// match the FinancialAnalysisResult interface the frontend already expects, so
// the UI needs no change when it stops calling the Firebase callable.
type AnalysisResult struct {
	Mode     string          `json:"mode"`
	Markdown string          `json:"markdown"`
	Summary  AnalysisSummary `json:"summary"`
	Usage    *AnalysisUsage  `json:"usage,omitempty"`
}

type AnalysisSummary struct {
	TotalTransactions         int               `json:"totalTransactions"`
	TotalTransactionsAnalyzed int               `json:"totalTransactionsAnalyzed"`
	AnalyzedDateRange         *DateRange        `json:"analyzedDateRange"`
	IncomeTotal               int64             `json:"incomeTotal"`
	ExpenseTotal              int64             `json:"expenseTotal"`
	NetBalance                int64             `json:"netBalance"`
	TopCategories             []CategorySummary `json:"topCategories"`
	MonthlySummaries          []MonthSummary    `json:"monthlySummaries"`
	SamplesUsed               SamplesUsed       `json:"samplesUsed"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type CategorySummary struct {
	Name       string  `json:"name"`
	Total      int64   `json:"total"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type MonthSummary struct {
	Month   string `json:"month"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
	Net     int64  `json:"net"`
}

// SamplesUsed reports how the prompt sample was assembled, which the UI shows so
// the user can see the analysis did not read every transaction.
type SamplesUsed struct {
	Recent          int `json:"recent"`
	LargestExpense  int `json:"largestExpense"`
	CategoryAnchors int `json:"categoryAnchors"`
	IncomeAnchors   int `json:"incomeAnchors"`
}

type AnalysisUsage struct {
	PromptTokens         int `json:"promptTokens"`
	CandidateTokens      int `json:"candidateTokens"`
	TotalTokens          int `json:"totalTokens"`
	RemainingDailyTokens int `json:"remainingDailyTokens"`
	DailyTokenLimit      int `json:"dailyTokenLimit"`
}

// topCategorySummaries converts breakdown rows for the UI table.
func topCategorySummaries(breakdown []transaction.CategoryBreakdown, n int) []CategorySummary {
	rows := topCategories(breakdown, n)
	out := make([]CategorySummary, 0, len(rows))
	for _, c := range rows {
		out = append(out, CategorySummary{
			Name: c.Category, Total: c.Amount, Count: c.Count, Percentage: c.Percentage,
		})
	}
	return out
}

// monthlySummaryFor aggregates one month of details.
func monthlySummaryFor(label string, details []transaction.Detail) MonthSummary {
	income, _ := transaction.SumByType(details, "INCOME")
	expense, _ := transaction.SumByType(details, "EXPENSE")
	return MonthSummary{Month: label, Income: income, Expense: expense, Net: income - expense}
}

// monthNames indexes 1-12; index 0 is unused so the month number maps directly.
var monthNames = []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// indonesianMonthLabel renders "Januari 2026" for the monthly table.
func indonesianMonthLabel(t time.Time) string {
	t = t.In(datetime.Location())
	return monthNames[int(t.Month())] + " " + strconv.Itoa(t.Year())
}
