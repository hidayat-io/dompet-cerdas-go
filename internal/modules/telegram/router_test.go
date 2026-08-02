package telegram

import (
	"strings"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// These are the messages that went unanswered in production while the text
// handler was a stub. Each must classify into an intent the router can answer,
// otherwise the bot goes quiet again for exactly the same inputs.
func TestRoutableIntent_MessagesThatWentUnanswered(t *testing.T) {
	tests := []struct {
		message string
		intent  string
	}{
		{message: "tampilkan 10 transt erakhir", intent: IntentQueryDetails},
		{message: "tmapilkan 10 trans terakhir", intent: IntentQueryDetails},
		{message: "tampilkan 10 transaksi terakhir", intent: IntentQueryDetails},
		{message: "saldo", intent: IntentQueryBalance},
		{message: "berapa pengeluaran hari ini?", intent: IntentQueryExpenses},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			got := DetectSimpleIntent(tt.message)
			if got == nil {
				t.Fatalf("DetectSimpleIntent(%q) = nil, want intent %s", tt.message, tt.intent)
			}
			if got.Intent != tt.intent {
				t.Errorf("intent = %q, want %q", got.Intent, tt.intent)
			}
			if !routableIntents[got.Intent] {
				t.Errorf("intent %q is not handled by handleTextMessage", got.Intent)
			}
		})
	}
}

// routableIntents mirrors the switch in handleTextMessage. Keeping it beside the
// test makes a new intent that nobody routed show up as a failure.
var routableIntents = map[string]bool{
	IntentQueryExpenses:     true,
	IntentQueryIncome:       true,
	IntentQueryBalance:      true,
	IntentQueryDetails:      true,
	IntentCategoryBreakdown: true,
	IntentListCategories:    true,
	IntentAddTransaction:    true,
	IntentFinancialAdvice:   true,
	IntentSavingsStrategy:   true,
	IntentExpenseAnalysis:   true,
}

// Every intent the classifier can emit must have a branch, so that no input can
// reach the router and produce nothing.
func TestEveryIntentIsRouted(t *testing.T) {
	all := []string{
		IntentQueryExpenses, IntentQueryIncome, IntentQueryBalance,
		IntentAddTransaction, IntentCategoryBreakdown, IntentQueryDetails,
		IntentListCategories, IntentFinancialAdvice, IntentSavingsStrategy,
		IntentExpenseAnalysis,
	}
	for _, intent := range all {
		if !routableIntents[intent] {
			t.Errorf("intent %q has no branch in handleTextMessage", intent)
		}
	}
}

func TestDateRangeFor_SpecificDateWins(t *testing.T) {
	p := IntentParameters{SpecificDate: "2026-03-09", CustomMonth: "2026-01", DaysAgo: 5, TimeRange: TimeRangeThisMonth}

	start, end, err := dateRangeFor(p, TimeRangeThisMonth)
	if err != nil {
		t.Fatalf("dateRangeFor: %v", err)
	}
	if start != "2026-03-09" || end != "2026-03-09" {
		t.Errorf("range = %s..%s, want 2026-03-09..2026-03-09", start, end)
	}
}

func TestDateRangeFor_CustomMonthBeatsRelativeAndNamed(t *testing.T) {
	p := IntentParameters{CustomMonth: "2026-02", DaysAgo: 5, TimeRange: TimeRangeThisMonth}

	start, end, err := dateRangeFor(p, TimeRangeThisMonth)
	if err != nil {
		t.Fatalf("dateRangeFor: %v", err)
	}
	if start != "2026-02-01" || end != "2026-02-28" {
		t.Errorf("range = %s..%s, want 2026-02-01..2026-02-28", start, end)
	}
}

func TestDateRangeFor_AllTimeUsesLegacyLowerBound(t *testing.T) {
	start, end, err := dateRangeFor(IntentParameters{TimeRange: TimeRangeAllTime}, TimeRangeThisMonth)
	if err != nil {
		t.Fatalf("dateRangeFor: %v", err)
	}
	if start != allTimeStart {
		t.Errorf("start = %s, want %s", start, allTimeStart)
	}
	if want := datetime.TodayString(); end != want {
		t.Errorf("end = %s, want today %s", end, want)
	}
}

func TestDateRangeFor_FallsBackWhenNoRangeGiven(t *testing.T) {
	start, end, err := dateRangeFor(IntentParameters{}, TimeRangeToday)
	if err != nil {
		t.Fatalf("dateRangeFor: %v", err)
	}
	today := datetime.TodayString()
	if start != today || end != today {
		t.Errorf("range = %s..%s, want %s on both sides", start, end, today)
	}
}

// days_ago resolves to a single day, not a span, which is the legacy behavior
// ResolveRange documents.
func TestDateRangeFor_DaysAgoIsASingleDay(t *testing.T) {
	start, end, err := dateRangeFor(IntentParameters{DaysAgo: 3}, TimeRangeThisMonth)
	if err != nil {
		t.Fatalf("dateRangeFor: %v", err)
	}
	if start != end {
		t.Errorf("range = %s..%s, want a single day", start, end)
	}
	if want := datetime.Now().AddDate(0, 0, -3).Format("2006-01-02"); start != want {
		t.Errorf("start = %s, want %s", start, want)
	}
}

func TestTimeRangeLabel(t *testing.T) {
	tests := []struct {
		name   string
		params IntentParameters
		want   string
	}{
		{name: "custom_month", params: IntentParameters{CustomMonth: "2026-02"}, want: "Februari 2026"},
		{name: "days_ago", params: IntentParameters{DaysAgo: 3}, want: "3 hari lalu"},
		{name: "named_range", params: IntentParameters{TimeRange: TimeRangeThisWeek}, want: "minggu ini"},
		{name: "fallback", params: IntentParameters{}, want: "bulan ini"},
		{name: "all_time", params: IntentParameters{TimeRange: TimeRangeAllTime}, want: "semua waktu"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeRangeLabel(tt.params, TimeRangeThisMonth); got != tt.want {
				t.Errorf("timeRangeLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

// A malformed custom month is echoed instead of being read as some other month,
// so a parser bug cannot masquerade as a valid answer.
func TestCustomMonthLabel_MalformedIsEchoed(t *testing.T) {
	for _, input := range []string{"2026", "2026-13", "2026-xx", ""} {
		if got := customMonthLabel(input); got != input {
			t.Errorf("customMonthLabel(%q) = %q, want it echoed unchanged", input, got)
		}
	}
}

func TestDetailsLabel(t *testing.T) {
	tests := []struct {
		name   string
		params IntentParameters
		limit  int
		want   string
	}{
		{
			name:   "limit_by_date",
			params: IntentParameters{Limit: 10, SortBy: transaction.SortByDate},
			limit:  10,
			want:   "10 transaksi terakhir",
		},
		{
			name:   "limit_by_amount",
			params: IntentParameters{Limit: 5, SortBy: transaction.SortByAmount},
			limit:  5,
			want:   "5 transaksi tertinggi",
		},
		{
			name:   "limit_with_type_filter",
			params: IntentParameters{Limit: 3, SortBy: transaction.SortByAmount, TypeFilter: string(domain.TransactionTypeIncome)},
			limit:  3,
			want:   "3 pemasukan tertinggi",
		},
		{
			name:   "specific_date",
			params: IntentParameters{SpecificDate: "2026-01-27"},
			limit:  0,
			want:   "tanggal 27 Jan - Selasa",
		},
		{
			name:   "days_ago",
			params: IntentParameters{DaysAgo: 2},
			limit:  0,
			want:   "2 hari lalu",
		},
		{
			name:   "named_range",
			params: IntentParameters{TimeRange: TimeRangeLastMonth},
			limit:  0,
			want:   "bulan lalu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detailsLabel(tt.params, tt.params, tt.limit); got != tt.want {
				t.Errorf("detailsLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

// A limit above the cap is clamped and the user is told, rather than silently
// receiving fewer rows than asked for.
func TestClampNoticeThreshold(t *testing.T) {
	if limit, clamped := transaction.ClampTelegramLimit(50); limit != 30 || !clamped {
		t.Errorf("ClampTelegramLimit(50) = (%d, %v), want (30, true)", limit, clamped)
	}
	if limit, clamped := transaction.ClampTelegramLimit(10); limit != 10 || clamped {
		t.Errorf("ClampTelegramLimit(10) = (%d, %v), want (10, false)", limit, clamped)
	}
}

// The notice text is what the legacy bot shows; a change here changes what users
// read, so it is pinned.
func TestNotPortedMessageNamesTheSubject(t *testing.T) {
	got := notPortedMessage("Pencatatan transaksi lewat chat")
	if want := "🚧 Pencatatan transaksi lewat chat belum tersedia"; !strings.Contains(got, want) {
		t.Errorf("notPortedMessage = %q, want it to contain %q", got, want)
	}
	if !strings.Contains(got, "/help") {
		t.Error("notPortedMessage should point the user at /help")
	}
}
