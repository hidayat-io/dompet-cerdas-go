package telegram

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// Intent detection patterns ported from nluService.ts:104-107.
var (
	TransactionQueryPattern = regexp.MustCompile(`(?i)trans(?:aksi)?s?|transs|transaski|transsaksi|tranaksi|transactions?|txs?`)
	DetailQueryPattern      = regexp.MustCompile(`(?i)detail|rincian|apa\s+aja|apa\s+saja|list|tampilkan|lihat|show|tunjukkan`)
	RankingQueryPattern     = regexp.MustCompile(`(?i)\b(top|last|latest|terakhir|tertinggi|terbesar|terbanyak|highest|biggest|largest)\b`)
	LimitQueryPattern       = regexp.MustCompile(`(?i)\b(top|last|latest)\s+\d+\b|(?:^|\s)\d+\s+(?:trans(?:aksi)?s?|transs|transaski|transsaksi|tranaksi|transactions?|txs?|item|data|pengeluaran)\b`)
	BalancePattern          = regexp.MustCompile(`(?i)^(berapa\s+)?(saldo|balance|sisa\s+uang)(\s+(sekarang|saya|kamu|aku|gw))?(\s+berapa)?$`)
)

// Supporting patterns, all precompiled at package level.
//
// expenseOrIncomeWord reproduces /\bpengeluaran|pemasukan\b/i from
// nluService.ts:127 verbatim. Note the alternation binds loosely there: the
// left branch is `\bpengeluaran` and the right is `pemasukan\b`, so the two
// sides have asymmetric anchoring. Preserved as-is for parity.
var (
	expenseOrIncomeWord   = regexp.MustCompile(`(?i)\bpengeluaran|pemasukan\b`)
	typoTransactionWord   = regexp.MustCompile(`(?i)\b(transaski|transsaksi|tranaksi)\b`)
	canonicalRecentQuery  = regexp.MustCompile(`(?i)^(last\s+\d+\s+trans|\d+\s+(transaksi|trans)\s+terakhir|top\s+\d+\s+transaksi(?:\s+bulan\s+ini)?)$`)
	thisMonthPattern      = regexp.MustCompile(`(?i)bulan\s+ini|bln\s+ini|bulan\s+ni|this\s+month|thins\s+month`)
	savingsStrategyIntent = regexp.MustCompile(`(?i)tips|strategi|saran.*hemat|cara.*hemat|biar.*hemat|agar.*hemat`)
	expenseAnalysisIntent = regexp.MustCompile(`(?i)kurangi|potong|cut|bisa.*turun|bisa.*hemat|tanpa suffering|suffering`)

	dateMonthPattern = regexp.MustCompile(`(?i)\b(\d{1,2})\s+(jan|januari|feb|februari|mar|maret|apr|april|mei|jun|juni|jul|juli|agt|agustus|sep|september|okt|oktober|nov|november|des|desember)\b`)
	tglPattern       = regexp.MustCompile(`(?i)\b(?:tgl|tanggal)\s+(\d{1,2})\b`)

	balanceMentionPattern = regexp.MustCompile(`(?i)saldo|balance|sisa\s+uang`)

	listCategoriesPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)kategori\s+apa\s*(aja|saja)?`),
		regexp.MustCompile(`(?i)apa\s+(aja|saja)\s+kategori`),
		regexp.MustCompile(`(?i)ada\s+kategori\s+apa`),
		regexp.MustCompile(`(?i)list\s+kategori|daftar\s+kategori`),
		regexp.MustCompile(`(?i)kategori\s+yang\s+ada`),
		regexp.MustCompile(`(?i)kategori\s+tersedia`),
	}

	breakdownPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)breakdown|boros|paling\s+banyak`),
		regexp.MustCompile(`(?i)pengeluaran\s+(per|tiap)\s+kategori`),
		regexp.MustCompile(`(?i)spending\s+(per|by)\s+category`),
	}

	todayPattern     = regexp.MustCompile(`(?i)hari\s+ini|today`)
	yesterdayPattern = regexp.MustCompile(`(?i)kemarin|yesterday`)
	thisWeekPattern  = regexp.MustCompile(`(?i)minggu\s+ini|this\s+week`)
	lastMonthPattern = regexp.MustCompile(`(?i)bulan\s+lalu|last\s+month`)
	nDaysAgoPattern  = regexp.MustCompile(`(?i)\d+\s+hari\s+ter[a-z]+`)

	incomeTypePattern  = regexp.MustCompile(`(?i)\bpemasukan|income\b`)
	expenseTypePattern = regexp.MustCompile(`(?i)\bpengeluaran|expense|expenses\b`)

	categoryFilterPattern = regexp.MustCompile(`(?i)kategori\s+(\w+)`)

	highestTxPattern = regexp.MustCompile(`(?i)(?:^|\s)(\d+)\s+(?:transaksi|transaski|transsaksi|tranaksi|trans|pengeluaran|pemasukan)?\s*(?:tertinggi|terbesar|terbanyak|highest|biggest|largest)`)
	topNPattern      = regexp.MustCompile(`(?i)top\s+(\d+)`)
	recentTxPattern  = regexp.MustCompile(`(?i)(?:^|\s)(\d+)\s+(?:trans(?:aksi)?s?|transs|transaski|transsaksi|tranaksi|transactions?|txs?|item|data|pengeluaran|pemasukan)?\s*terakhir|last\s+(\d+)|(?:^|\s)(\d+)\s+(?:last|latest)`)

	typedRankingTopN     = regexp.MustCompile(`(?i)\btop\s+\d+\b`)
	typedRankingNTyped   = regexp.MustCompile(`(?i)(?:^|\s)\d+\s+(?:pemasukan|pengeluaran|income|expense|expenses)\s+(?:terakhir|tertinggi|terbesar|terbanyak)\b`)
	typedFlowWord        = regexp.MustCompile(`(?i)\b(?:pemasukan|pengeluaran|income|expense|expenses)\b`)
	typedSuperlativeWord = regexp.MustCompile(`(?i)\b(?:tertinggi|terbesar|terbanyak|highest|biggest|largest)\b`)
	anyDigit             = regexp.MustCompile(`\b\d+\b`)
	incomeWordStrict     = regexp.MustCompile(`(?i)\b(pemasukan|income)\b`)

	highestFlowPattern = regexp.MustCompile(`(?i)(?:^|\s)(\d+)\s+(?:pemasukan|pengeluaran|income|expense|expenses)?\s*(?:tertinggi|terbesar|terbanyak|highest|biggest|largest)`)
	recentFlowPattern  = regexp.MustCompile(`(?i)(?:^|\s)(\d+)\s+(?:pemasukan|pengeluaran|income|expense|expenses)?\s*terakhir|last\s+(\d+)|(?:^|\s)(\d+)\s+(?:last|latest)`)

	expenseWordPattern = regexp.MustCompile(`(?i)pengeluaran`)
	incomeWordPattern  = regexp.MustCompile(`(?i)pemasukan`)
	queryIndicator     = regexp.MustCompile(`(?i)(berapa|total|ada|apa|cek|check)`)
	timeIndicator      = regexp.MustCompile(`(?i)(hari|kemarin|minggu|bulan|today|yesterday|week|month)`)
)

// Intent identifiers matching the IntentType union in nluService.ts.
const (
	IntentQueryExpenses     = "query_expenses"
	IntentQueryIncome       = "query_income"
	IntentQueryBalance      = "query_balance"
	IntentAddTransaction    = "add_transaction"
	IntentCategoryBreakdown = "category_breakdown"
	IntentQueryDetails      = "query_details"
	IntentListCategories    = "list_categories"
	IntentFinancialAdvice   = "financial_advice"
	IntentSavingsStrategy   = "savings_strategy"
	IntentExpenseAnalysis   = "expense_analysis"
	IntentUnknown           = "unknown"
)

// Time range identifiers accepted by datetime.ResolveRange.
const (
	TimeRangeToday     = "today"
	TimeRangeYesterday = "yesterday"
	TimeRangeThisWeek  = "this_week"
	TimeRangeLastWeek  = "last_week"
	TimeRangeThisMonth = "this_month"
	TimeRangeLastMonth = "last_month"
	// TimeRangeAllTime is not produced by intent detection. The bot substitutes
	// it when a limit-only query arrives with no time constraint, so that
	// "10 transaksi terakhir" searches history instead of the current month
	// (bot/index.ts:1342-1344).
	TimeRangeAllTime = "all_time"
)

// IntentParameters carries the extracted query parameters for an intent.
type IntentParameters struct {
	TimeRange      string `json:"time_range,omitempty"`
	DaysAgo        int    `json:"days_ago,omitempty"`
	CustomMonth    string `json:"custom_month,omitempty"`
	SpecificDate   string `json:"specific_date,omitempty"`
	Amount         int64  `json:"amount,omitempty"`
	Description    string `json:"description,omitempty"`
	CategoryHint   string `json:"category_hint,omitempty"`
	CategoryFilter string `json:"category_filter,omitempty"`
	TypeFilter     string `json:"type_filter,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	SortBy         string `json:"sort_by,omitempty"`
}

// ParsedIntent is the result of intent classification.
type ParsedIntent struct {
	Intent     string           `json:"intent"`
	Confidence string           `json:"confidence,omitempty"`
	Parameters IntentParameters `json:"parameters,omitempty"`
}

// ShouldPreferAIIntentParsing decides whether a locally-classified intent should
// be re-parsed by the LLM instead of trusted directly. Ported from
// nluService.ts:123-151.
//
// Typo variants of "transaksi" always route to the LLM. That branch exists
// because production misparsed queries like "show 10 last transs" as a
// transaction to save (fixed in v2.8.10); removing it reintroduces that bug.
func ShouldPreferAIIntentParsing(message string, simpleIntent *ParsedIntent) bool {
	lower := strings.TrimSpace(strings.ToLower(message))

	hasDetailSignals := DetailQueryPattern.MatchString(lower) || TransactionQueryPattern.MatchString(lower)
	hasRankingSignals := RankingQueryPattern.MatchString(lower) || LimitQueryPattern.MatchString(lower)
	hasExpenseOrIncomeWord := expenseOrIncomeWord.MatchString(lower)

	if typoTransactionWord.MatchString(lower) {
		return true
	}

	if simpleIntent == nil {
		return false
	}

	switch simpleIntent.Intent {
	case IntentQueryExpenses, IntentQueryIncome:
		if hasRankingSignals || hasDetailSignals {
			return true
		}
	case IntentCategoryBreakdown:
		if hasRankingSignals {
			return true
		}
	case IntentQueryDetails:
		if !canonicalRecentQuery.MatchString(lower) && (hasRankingSignals || hasExpenseOrIncomeWord) {
			return true
		}
	}

	return false
}

// adviceKeywords mirrors ADVICE_KEYWORDS in nluService.ts:77-102.
// Matching is substring-based (lower.includes), not word-boundary, exactly as
// the legacy implementation does.
var adviceKeywords = []string{
	"gimana",
	"bagaimana",
	"analisa",
	"analisis",
	"analyze",
	"saran",
	"tips",
	"strategi",
	"strategy",
	"rekomendasi",
	"recommend",
	"hemat",
	"menghemat",
	"saving",
	"bisa dikurangi",
	"kurangi",
	"potong",
	"cut",
	"sebaiknya",
	"insight",
	"advice",
	"advisor",
	"suffering",
	"tanpa suffering",
}

// ContainsAdviceKeywords reports whether the message contains any advice keyword.
func ContainsAdviceKeywords(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range adviceKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isThisMonthPhrase detects "this month" phrasings. The legacy pattern
// (nluService.ts:120) includes the typo variant "thins month"; it is preserved.
func isThisMonthPhrase(lower string) bool {
	return thisMonthPattern.MatchString(lower)
}

// ClassifyAdviceIntent maps an advice-flavoured message to its specific intent.
// Ported from the fast path in nluService.ts:644-670. Callers must gate this
// behind ContainsAdviceKeywords.
func ClassifyAdviceIntent(message string) *ParsedIntent {
	lower := strings.ToLower(message)

	intent := IntentFinancialAdvice
	switch {
	case savingsStrategyIntent.MatchString(lower):
		intent = IntentSavingsStrategy
	case expenseAnalysisIntent.MatchString(lower):
		intent = IntentExpenseAnalysis
	}

	return &ParsedIntent{
		Intent:     intent,
		Confidence: "high",
		Parameters: IntentParameters{TimeRange: TimeRangeThisMonth},
	}
}

// resolveTimeRange maps time phrases to a range identifier. When includeNDaysAgo
// is set, "N hari terakhir" maps to last_week, matching the legacy behavior
// where that phrase means "the last 7 days including today".
func resolveTimeRange(lower string, includeNDaysAgo bool) string {
	if includeNDaysAgo && nDaysAgoPattern.MatchString(lower) {
		return TimeRangeLastWeek
	}
	switch {
	case todayPattern.MatchString(lower):
		return TimeRangeToday
	case yesterdayPattern.MatchString(lower):
		return TimeRangeYesterday
	case thisWeekPattern.MatchString(lower):
		return TimeRangeThisWeek
	case isThisMonthPhrase(lower):
		return TimeRangeThisMonth
	case lastMonthPattern.MatchString(lower):
		return TimeRangeLastMonth
	}
	return ""
}

// firstSubmatchInt returns the first non-empty capture group parsed as an int.
// The legacy code relies on this because its recent-transaction regex has three
// alternative capture positions and only one is ever populated.
func firstSubmatchInt(m []string) int {
	for i := 1; i < len(m); i++ {
		if m[i] != "" {
			n, err := strconv.Atoi(m[i])
			if err == nil {
				return n
			}
		}
	}
	return 0
}

// DetectSimpleIntent runs the local rule-based classifier. It returns nil when
// the message cannot be classified without the LLM. Ported from
// nluService.ts:153-521. Branch order is significant and preserved.
func DetectSimpleIntent(message string) *ParsedIntent {
	return detectSimpleIntentAt(message, datetime.Now())
}

func detectSimpleIntentAt(message string, ref time.Time) *ParsedIntent {
	lower := strings.TrimSpace(strings.ToLower(message))
	ref = ref.In(datetime.Location())

	monthMap := map[string]string{
		"jan": "01", "januari": "01",
		"feb": "02", "februari": "02",
		"mar": "03", "maret": "03",
		"apr": "04", "april": "04",
		"mei": "05",
		"jun": "06", "juni": "06",
		"jul": "07", "juli": "07",
		"agt": "08", "agustus": "08",
		"sep": "09", "september": "09",
		"okt": "10", "oktober": "10",
		"nov": "11", "november": "11",
		"des": "12", "desember": "12",
	}

	var specificDate string
	if m := dateMonthPattern.FindStringSubmatch(lower); len(m) > 0 {
		day := m[1]
		if len(day) == 1 {
			day = "0" + day
		}
		specificDate = strconv.Itoa(ref.Year()) + "-" + monthMap[strings.ToLower(m[2])] + "-" + day
	}
	if specificDate == "" {
		if m := tglPattern.FindStringSubmatch(lower); len(m) > 0 {
			day := m[1]
			if len(day) == 1 {
				day = "0" + day
			}
			specificDate = ref.Format("2006-01") + "-" + day
		}
	}

	if BalancePattern.MatchString(lower) {
		return &ParsedIntent{Intent: IntentQueryBalance, Confidence: "high"}
	}

	if balanceMentionPattern.MatchString(lower) {
		if tr := resolveTimeRange(lower, false); tr != "" {
			return &ParsedIntent{
				Intent:     IntentQueryBalance,
				Confidence: "high",
				Parameters: IntentParameters{TimeRange: tr},
			}
		}
	}

	for _, p := range listCategoriesPatterns {
		if p.MatchString(lower) {
			return &ParsedIntent{Intent: IntentListCategories, Confidence: "high"}
		}
	}

	for _, p := range breakdownPatterns {
		if p.MatchString(lower) {
			tr := resolveTimeRange(lower, false)
			if tr == "" {
				tr = TimeRangeThisMonth
			}
			return &ParsedIntent{
				Intent:     IntentCategoryBreakdown,
				Confidence: "high",
				Parameters: IntentParameters{TimeRange: tr},
			}
		}
	}

	if TransactionQueryPattern.MatchString(lower) || DetailQueryPattern.MatchString(lower) {
		timeRange := resolveTimeRange(lower, true)

		var typeFilter string
		if incomeTypePattern.MatchString(lower) {
			typeFilter = "INCOME"
		} else if expenseTypePattern.MatchString(lower) {
			typeFilter = "EXPENSE"
		}

		var categoryFilter string
		if m := categoryFilterPattern.FindStringSubmatch(lower); len(m) > 0 {
			categoryFilter = m[1]
		}
		if categoryFilter == "" {
			categoryFilter = matchKnownCategory(lower)
		}

		limit, sortBy := extractLimitAndSort(lower, highestTxPattern, recentTxPattern)

		// The legacy expression is:
		//   time_range: specific_date ? undefined : (limit ? time_range : (time_range || 'this_week'))
		// so the this_week default applies only when there is no limit and no
		// specific date.
		if specificDate != "" {
			timeRange = ""
		} else if limit == 0 && timeRange == "" {
			timeRange = TimeRangeThisWeek
		}

		return &ParsedIntent{
			Intent:     IntentQueryDetails,
			Confidence: "high",
			Parameters: IntentParameters{
				TimeRange:      timeRange,
				CategoryFilter: categoryFilter,
				TypeFilter:     typeFilter,
				Limit:          limit,
				SortBy:         sortBy,
				SpecificDate:   specificDate,
			},
		}
	}

	hasTypedRankingPattern := typedRankingTopN.MatchString(lower) ||
		typedRankingNTyped.MatchString(lower) ||
		(typedFlowWord.MatchString(lower) &&
			typedSuperlativeWord.MatchString(lower) &&
			anyDigit.MatchString(lower))

	if typedFlowWord.MatchString(lower) && hasTypedRankingPattern {
		typeFilter := "EXPENSE"
		if incomeWordStrict.MatchString(lower) {
			typeFilter = "INCOME"
		}

		limit, sortBy := extractLimitAndSort(lower, highestFlowPattern, recentFlowPattern)

		return &ParsedIntent{
			Intent:     IntentQueryDetails,
			Confidence: "high",
			Parameters: IntentParameters{
				TimeRange:  resolveTimeRange(lower, true),
				TypeFilter: typeFilter,
				Limit:      limit,
				SortBy:     sortBy,
			},
		}
	}

	if expenseWordPattern.MatchString(lower) {
		if queryIndicator.MatchString(lower) || timeIndicator.MatchString(lower) {
			tr := resolveTimeRange(lower, true)
			if tr == "" {
				tr = TimeRangeThisMonth
			}
			return &ParsedIntent{
				Intent:     IntentQueryExpenses,
				Confidence: "high",
				Parameters: IntentParameters{TimeRange: tr},
			}
		}
	}

	if incomeWordPattern.MatchString(lower) {
		if queryIndicator.MatchString(lower) || timeIndicator.MatchString(lower) {
			tr := resolveTimeRange(lower, false)
			if tr == "" {
				tr = TimeRangeThisMonth
			}
			return &ParsedIntent{
				Intent:     IntentQueryIncome,
				Confidence: "high",
				Parameters: IntentParameters{TimeRange: tr},
			}
		}
	}

	return nil
}

// knownCategoryPatterns maps direct category-name mentions to canonical names,
// ported from the categoryNames table in nluService.ts.
var knownCategoryPatterns = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`(?i)\bfood\b|\bmakan(an)?\b`), "Food"},
	{regexp.MustCompile(`(?i)\bshopping\b|\bbelanja\b`), "Shopping"},
	{regexp.MustCompile(`(?i)\bbill\b|\btagihan\b`), "Bill"},
	{regexp.MustCompile(`(?i)\btransport(asi)?\b|\bgojek\b|\bgrab\b`), "Transportation"},
	{regexp.MustCompile(`(?i)\bentertainment\b|\bhiburan\b`), "Entertainment"},
	{regexp.MustCompile(`(?i)\bhealth\b|\bkesehatan\b`), "Health"},
}

func matchKnownCategory(lower string) string {
	for _, c := range knownCategoryPatterns {
		if c.pattern.MatchString(lower) {
			return c.name
		}
	}
	return ""
}

// extractLimitAndSort resolves the requested item count and sort order.
// Precedence is superlative ("N tertinggi") then "top N", both sorting by
// amount, then "N terakhir" sorting by date.
func extractLimitAndSort(lower string, highest, recent *regexp.Regexp) (int, string) {
	if m := highest.FindStringSubmatch(lower); len(m) > 0 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, "amount"
		}
	}
	if m := topNPattern.FindStringSubmatch(lower); len(m) > 0 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, "amount"
		}
	}
	if m := recent.FindStringSubmatch(lower); len(m) > 0 {
		if n := firstSubmatchInt(m); n > 0 {
			return n, "date"
		}
	}
	return 0, ""
}
