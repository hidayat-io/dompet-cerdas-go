package domain

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ReceiptType classifies the kind of receipt scanned.
type ReceiptType string

const (
	ReceiptTypeRetail     ReceiptType = "retail"
	ReceiptTypeRestaurant ReceiptType = "restaurant"
	ReceiptTypeTransport  ReceiptType = "transport"
	ReceiptTypeBill       ReceiptType = "bill"
	ReceiptTypeOther      ReceiptType = "other"
)

// Confidence indicates how confident the AI is in its result.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// ConfidenceScore is the model's self-reported extraction confidence, 0-100.
// Models occasionally emit floats or quoted numbers, and a hard int would turn
// a fine receipt into a parse error; degrading to 0 instead fails toward the
// confirmation flow, since 0 can never open the auto-save gate.
type ConfidenceScore int

func (s *ConfidenceScore) UnmarshalJSON(data []byte) error {
	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		*s = ConfidenceScore(number)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		number, err := strconv.Atoi(strings.TrimSpace(str))
		if err != nil {
			*s = 0
			return nil
		}
		*s = ConfidenceScore(number)
		return nil
	}
	*s = 0
	return nil
}

// ReceiptData holds the AI-extracted data from a receipt image.
type ReceiptData struct {
	Merchant           string          `firestore:"merchant"                      json:"merchant"`
	TotalAmount        int64           `firestore:"totalAmount"                   json:"totalAmount"`
	Date               string          `firestore:"date"                          json:"date"` // YYYY-MM-DD
	Items              []string        `firestore:"items,omitempty"               json:"items,omitempty"`
	CategorySuggestion string          `firestore:"categorySuggestion"            json:"categorySuggestion"`
	ReceiptType        ReceiptType     `firestore:"receiptType"                   json:"receiptType"`
	Confidence         Confidence      `firestore:"confidence"                    json:"confidence"`
	ConfidenceScore    ConfidenceScore `firestore:"confidenceScore,omitempty"     json:"confidenceScore,omitempty"`
	Currency           string          `firestore:"currency"                      json:"currency"`
	Notes              string          `firestore:"notes,omitempty"               json:"notes,omitempty"`
	IsReceipt          bool            `firestore:"is_receipt,omitempty"          json:"is_receipt,omitempty"`
}

// WebAnalysisMode selects which AI analysis the user wants.
type WebAnalysisMode string

const (
	WebAnalysisModeHealth   WebAnalysisMode = "HEALTH"
	WebAnalysisModeSpending WebAnalysisMode = "SPENDING"
	WebAnalysisModeSavings  WebAnalysisMode = "SAVINGS"
)

// FinancialInsightsResult is the response from the Gemini financial advisor.
type FinancialInsightsResult struct {
	Text            string `json:"text"`
	PromptTokens    int    `json:"promptTokens"`
	CandidateTokens int    `json:"candidateTokens"`
	TotalTokens     int    `json:"totalTokens"`
}

// CategorySummary is a per-category spending breakdown in analysis results.
type CategorySummary struct {
	Name       string  `json:"name"`
	Total      int64   `json:"total"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

// MonthSummary is a per-month income/expense breakdown in analysis results.
type MonthSummary struct {
	Month   string `json:"month"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
	Net     int64  `json:"net"`
}

// SamplesUsed describes the sampling strategy used for transaction analysis.
type SamplesUsed struct {
	Recent          int `json:"recent"`
	LargestExpense  int `json:"largestExpense"`
	CategoryAnchors int `json:"categoryAnchors"`
	IncomeAnchors   int `json:"incomeAnchors"`
}

// AnalyzedDateRange is the date range covered by an analysis.
type AnalyzedDateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// WebAnalysisSummary contains the structured data portion of a web analysis.
type WebAnalysisSummary struct {
	TotalTransactions         int                `json:"totalTransactions"`
	TotalTransactionsAnalyzed int                `json:"totalTransactionsAnalyzed"`
	AnalyzedDateRange         *AnalyzedDateRange `json:"analyzedDateRange"` // nullable
	IncomeTotal               int64              `json:"incomeTotal"`
	ExpenseTotal              int64              `json:"expenseTotal"`
	NetBalance                int64              `json:"netBalance"`
	TopCategories             []CategorySummary  `json:"topCategories"`
	MonthlySummaries          []MonthSummary     `json:"monthlySummaries"`
	SamplesUsed               SamplesUsed        `json:"samplesUsed"`
}

// WebAnalysisUsage tracks AI token consumption for a web analysis request.
type WebAnalysisUsage struct {
	PromptTokens         int `json:"promptTokens"`
	CandidateTokens      int `json:"candidateTokens"`
	TotalTokens          int `json:"totalTokens"`
	RemainingDailyTokens int `json:"remainingDailyTokens"`
	DailyTokenLimit      int `json:"dailyTokenLimit"`
}

// WebFinancialAnalysisResult is the full response from web-based AI analysis.
type WebFinancialAnalysisResult struct {
	Mode     WebAnalysisMode    `json:"mode"`
	Markdown string             `json:"markdown"`
	Summary  WebAnalysisSummary `json:"summary"`
	Usage    WebAnalysisUsage   `json:"usage"`
}

// ParsedTransactionDraft is a single transaction extracted from natural language input.
type ParsedTransactionDraft struct {
	Amount       int64  `json:"amount"`
	Description  string `json:"description"`
	CategoryHint string `json:"category_hint,omitempty"`
	SourceText   string `json:"sourceText"`
}

// HybridTransactionParseResult is the output of the hybrid NLU+regex parser.
type HybridTransactionParseResult struct {
	Items               []ParsedTransactionDraft `json:"items"`
	UsedAI              bool                     `json:"usedAI"`
	Confidence          Confidence               `json:"confidence"`
	ClarificationNeeded string                   `json:"clarificationNeeded,omitempty"`
	// ConfidenceScore is only set by the receipt-photo path; the zero value
	// must never open the auto-save gate for other AI results.
	ConfidenceScore ConfidenceScore `json:"confidenceScore,omitempty"`
}
