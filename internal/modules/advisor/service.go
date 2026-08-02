package advisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
)

// InsightGenerator is the LLM leg, kept as an interface so data assembly and
// prompt construction can be tested without a network call.
type InsightGenerator interface {
	GenerateInsights(ctx context.Context, prompt string, systemInstruction string) (string, gemini.Usage, error)
}

// ErrUnavailable means no model is configured, so no analysis can be produced.
var ErrUnavailable = errors.New("financial analysis is unavailable")

// Analysis sample sizes, ported from the legacy callers.
const (
	healthSampleSize   = 50
	spendingSampleSize = 100
	detailFetchLimit   = 150
)

// Service produces the AI financial analyses.
type Service struct {
	accountService *account.Service
	accountRepo    *account.Repository
	generator      InsightGenerator
	quota          *QuotaManager
}

func NewService(
	accountService *account.Service,
	accountRepo *account.Repository,
	generator InsightGenerator,
	quota *QuotaManager,
) *Service {
	return &Service{
		accountService: accountService,
		accountRepo:    accountRepo,
		generator:      generator,
		quota:          quota,
	}
}

// Available reports whether analysis can run at all.
func (s *Service) Available() bool { return s.generator != nil }

// Analyze gathers the account's data, builds the prompt for the mode, and
// returns the model's answer.
//
// Quota is reserved only once the data is assembled and a call is actually about
// to happen, so a failure while reading Firestore does not burn one of the
// user's daily requests.
func (s *Service) Analyze(ctx context.Context, userID, accountID string, mode Mode) (*AnalysisResult, error) {
	if s.generator == nil {
		return nil, ErrUnavailable
	}

	ac, err := s.accountService.GetAccountContext(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}

	input, summary, err := s.buildInput(ctx, ac, mode)
	if err != nil {
		return nil, err
	}

	reservation, err := s.quota.Reserve(ctx, userID)
	if err != nil {
		return nil, err
	}

	text, usage, err := s.generator.GenerateInsights(ctx, BuildPrompt(mode, input), SystemInstruction)
	if err != nil {
		return nil, err
	}

	if err := s.quota.Commit(ctx, userID, reservation.DailyTokensUsed, usage.TotalTokens); err != nil {
		// The answer is already produced; failing the request now would waste it.
		slog.Error("advisor: failed to commit quota usage", "userId", userID, "error", err)
	}

	if IsOffTopic(text) {
		slog.Warn("advisor: model drifted off-topic, replacing answer", "userId", userID, "mode", mode)
		text = OffTopicReply
	}

	return &AnalysisResult{
		Mode:     string(mode),
		Markdown: text,
		Summary:  summary,
		Usage: &AnalysisUsage{
			PromptTokens:         usage.PromptTokens,
			CandidateTokens:      usage.CandidateTokens,
			TotalTokens:          usage.TotalTokens,
			RemainingDailyTokens: maxInt(0, DailyTokenLimit-(reservation.DailyTokensUsed+usage.TotalTokens)),
			DailyTokenLimit:      DailyTokenLimit,
		},
	}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// buildInput assembles the figures each prompt needs. The spending analysis
// works from a wider sample because it hunts for repeated small habits, which a
// top-50 cut would hide.
func (s *Service) buildInput(ctx context.Context, ac *account.Context, mode Mode) (PromptInput, AnalysisSummary, error) {
	now := datetime.Now()

	start, end, err := datetime.ResolveRange("this_month", now)
	if err != nil {
		return PromptInput{}, AnalysisSummary{}, err
	}

	details, err := transaction.QueryTransactions(ctx, s.accountService, s.accountRepo, ac, transaction.QueryParams{
		StartDate: start,
		EndDate:   end,
		SortBy:    transaction.SortByAmount,
		Limit:     detailFetchLimit,
	})
	if err != nil {
		return PromptInput{}, AnalysisSummary{}, err
	}

	expenseTotal, expenseCount := transaction.SumByType(details, domain.TransactionTypeExpense)
	incomeTotal, _ := transaction.SumByType(details, domain.TransactionTypeIncome)
	breakdown := transaction.BuildCategoryBreakdown(details)

	sampleSize := healthSampleSize
	if mode == ModeSpending {
		sampleSize = spendingSampleSize
	}
	selected := SelectRelevantTransactions(details, sampleSize)

	input := PromptInput{
		PeriodLabel:  "bulan ini",
		ExpenseTotal: expenseTotal,
		ExpenseCount: expenseCount,
		Breakdown:    breakdown,
		Transactions: selected,
		Patterns:     DetectSpendingPatterns(selected),
	}

	summary := AnalysisSummary{
		TotalTransactions:         len(details),
		TotalTransactionsAnalyzed: len(selected),
		AnalyzedDateRange:         &DateRange{Start: start, End: end},
		IncomeTotal:               incomeTotal,
		ExpenseTotal:              expenseTotal,
		NetBalance:                incomeTotal - expenseTotal,
		TopCategories:             topCategorySummaries(breakdown, 5),
		SamplesUsed:               countSamples(details, selected),
	}

	months, err := s.monthlySummaries(ctx, ac, now, 3)
	if err != nil {
		return PromptInput{}, AnalysisSummary{}, err
	}
	summary.MonthlySummaries = months

	if mode == ModeSpending {
		input.CategoryStats = BuildCategoryStats(breakdown, details)
		return input, summary, nil
	}

	// The health overview also compares against last month and reports the
	// running balance; the other two modes do not use either.
	if mode == ModeHealth {
		lastStart, lastEnd, err := datetime.ResolveRange("last_month", now)
		if err != nil {
			return PromptInput{}, AnalysisSummary{}, err
		}

		lastDetails, err := transaction.QueryTransactions(ctx, s.accountService, s.accountRepo, ac, transaction.QueryParams{
			StartDate: lastStart,
			EndDate:   lastEnd,
		})
		if err != nil {
			return PromptInput{}, AnalysisSummary{}, err
		}
		input.LastMonthTotal, _ = transaction.SumByType(lastDetails, domain.TransactionTypeExpense)

		allDetails, err := transaction.QueryTransactions(ctx, s.accountService, s.accountRepo, ac, transaction.QueryParams{
			StartDate: "2020-01-01",
			EndDate:   datetime.TodayString(),
		})
		if err != nil {
			return PromptInput{}, AnalysisSummary{}, err
		}
		input.Balance = transaction.NetBalance(allDetails)
		summary.NetBalance = input.Balance
	}

	return input, summary, nil
}

// monthlySummaries aggregates the last n months, newest first, for the trend
// table on the advisor screen.
func (s *Service) monthlySummaries(ctx context.Context, ac *account.Context, now time.Time, n int) ([]MonthSummary, error) {
	summaries := make([]MonthSummary, 0, n)

	for i := 0; i < n; i++ {
		month := now.AddDate(0, -i, 0)
		start, end, err := datetime.ResolveRange("custom_month:"+month.Format("2006-01"), now)
		if err != nil {
			return nil, err
		}

		details, err := transaction.QueryTransactions(ctx, s.accountService, s.accountRepo, ac, transaction.QueryParams{
			StartDate: start,
			EndDate:   end,
		})
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, monthlySummaryFor(indonesianMonthLabel(month), details))
	}

	return summaries, nil
}

// countSamples reports how the prompt sample was composed. Income anchors are
// always zero: SelectRelevantTransactions analyses expenses only.
func countSamples(all []transaction.Detail, selected []AnalysisTransaction) SamplesUsed {
	expenses := 0
	categories := map[string]bool{}
	for _, d := range all {
		if d.Type == domain.TransactionTypeExpense {
			expenses++
			categories[d.Category] = true
		}
	}

	return SamplesUsed{
		Recent:          minInt(selectMostRecent, expenses),
		LargestExpense:  minInt(selectTopByAmount, expenses),
		CategoryAnchors: len(categories),
		IncomeAnchors:   0,
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ModeFromString maps an API mode value onto a Mode, rejecting anything else.
func ModeFromString(value string) (Mode, error) {
	switch Mode(value) {
	case ModeHealth, ModeSavings, ModeSpending:
		return Mode(value), nil
	default:
		return "", fmt.Errorf("unknown analysis mode %q", value)
	}
}
