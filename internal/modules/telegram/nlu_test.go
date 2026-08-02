package telegram

import (
	"testing"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

func TestShouldPreferAIIntentParsing_Fixtures(t *testing.T) {
	const fixture = "nlu_routing.json"

	var data struct {
		Cases []struct {
			Message      string        `json:"message"`
			SimpleIntent *ParsedIntent `json:"simpleIntent"`
			Output       bool          `json:"output"`
		} `json:"cases"`
	}

	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		got := ShouldPreferAIIntentParsing(c.Message, c.SimpleIntent)
		if got != c.Output {
			t.Errorf("ShouldPreferAIIntentParsing(%q, %q) = %v, want %v",
				c.Message, c.SimpleIntent.Intent, got, c.Output)
		}
	}
}

// TestDetectSimpleIntent_Fixtures replays parseIntent output captured from the
// legacy implementation. Cases the legacy classifier could not resolve locally
// are recorded with intent "unknown"; the Go local classifier must return nil
// there so the caller falls through to the LLM.
//
// Advice intents are handled by a separate fast path in parseIntent that runs
// before detectSimpleIntent, so they are asserted via ClassifyAdviceIntent.
//
// The fixture's specific_date values were computed from the extraction-time
// clock, so the reference time is taken from generatedAt rather than time.Now().
func TestDetectSimpleIntent_Fixtures(t *testing.T) {
	const fixture = "nlu_intent.json"

	var data struct {
		GeneratedAt string `json:"generatedAt"`
		Cases       []struct {
			Message string `json:"message"`
			Output  struct {
				Intent     string `json:"intent"`
				Confidence string `json:"confidence"`
				Parameters struct {
					TimeRange      string `json:"time_range"`
					SpecificDate   string `json:"specific_date"`
					CategoryFilter string `json:"category_filter"`
					TypeFilter     string `json:"type_filter"`
					Limit          int    `json:"limit"`
					SortBy         string `json:"sort_by"`
				} `json:"parameters"`
			} `json:"output"`
		} `json:"cases"`
	}

	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	ref, err := time.Parse(time.RFC3339, data.GeneratedAt)
	if err != nil {
		t.Fatalf("invalid generatedAt %q: %v", data.GeneratedAt, err)
	}
	ref = ref.In(datetime.Location())

	for _, c := range data.Cases {
		want := c.Output

		if ContainsAdviceKeywords(c.Message) {
			got := ClassifyAdviceIntent(c.Message)
			if got.Intent != want.Intent {
				t.Errorf("ClassifyAdviceIntent(%q).Intent = %q, want %q",
					c.Message, got.Intent, want.Intent)
			}
			if got.Parameters.TimeRange != want.Parameters.TimeRange {
				t.Errorf("ClassifyAdviceIntent(%q).TimeRange = %q, want %q",
					c.Message, got.Parameters.TimeRange, want.Parameters.TimeRange)
			}
			continue
		}

		got := detectSimpleIntentAt(c.Message, ref)

		if want.Intent == IntentUnknown {
			if got != nil {
				t.Errorf("detectSimpleIntent(%q) = %+v, want nil (legacy deferred to LLM)",
					c.Message, got)
			}
			continue
		}

		if got == nil {
			t.Errorf("detectSimpleIntent(%q) = nil, want intent %q", c.Message, want.Intent)
			continue
		}

		if got.Intent != want.Intent {
			t.Errorf("detectSimpleIntent(%q).Intent = %q, want %q", c.Message, got.Intent, want.Intent)
		}
		if got.Confidence != want.Confidence {
			t.Errorf("detectSimpleIntent(%q).Confidence = %q, want %q", c.Message, got.Confidence, want.Confidence)
		}
		if got.Parameters.TimeRange != want.Parameters.TimeRange {
			t.Errorf("detectSimpleIntent(%q).TimeRange = %q, want %q",
				c.Message, got.Parameters.TimeRange, want.Parameters.TimeRange)
		}
		if got.Parameters.SpecificDate != want.Parameters.SpecificDate {
			t.Errorf("detectSimpleIntent(%q).SpecificDate = %q, want %q",
				c.Message, got.Parameters.SpecificDate, want.Parameters.SpecificDate)
		}
		if got.Parameters.CategoryFilter != want.Parameters.CategoryFilter {
			t.Errorf("detectSimpleIntent(%q).CategoryFilter = %q, want %q",
				c.Message, got.Parameters.CategoryFilter, want.Parameters.CategoryFilter)
		}
		if got.Parameters.TypeFilter != want.Parameters.TypeFilter {
			t.Errorf("detectSimpleIntent(%q).TypeFilter = %q, want %q",
				c.Message, got.Parameters.TypeFilter, want.Parameters.TypeFilter)
		}
		if got.Parameters.Limit != want.Parameters.Limit {
			t.Errorf("detectSimpleIntent(%q).Limit = %d, want %d",
				c.Message, got.Parameters.Limit, want.Parameters.Limit)
		}
		if got.Parameters.SortBy != want.Parameters.SortBy {
			t.Errorf("detectSimpleIntent(%q).SortBy = %q, want %q",
				c.Message, got.Parameters.SortBy, want.Parameters.SortBy)
		}
	}
}

func TestClassifyAdviceIntent(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "tips_to_savings", message: "tips hemat bulan depan", want: IntentSavingsStrategy},
		{name: "strategi_to_savings", message: "strategi hemat", want: IntentSavingsStrategy},
		{name: "saran_hemat_to_savings", message: "saran biar lebih hemat", want: IntentSavingsStrategy},
		{name: "kurangi_to_analysis", message: "kategori mana yang bisa dikurangi", want: IntentExpenseAnalysis},
		{name: "potong_to_analysis", message: "gimana caranya potong pengeluaran", want: IntentExpenseAnalysis},
		{name: "gimana_to_general", message: "gimana keuanganku bulan ini", want: IntentFinancialAdvice},
		{name: "analisa_to_general", message: "analisa pengeluaran aku", want: IntentFinancialAdvice},
		{name: "insight_to_general", message: "kasih insight dong", want: IntentFinancialAdvice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyAdviceIntent(tt.message)
			if got.Intent != tt.want {
				t.Errorf("ClassifyAdviceIntent(%q).Intent = %q, want %q", tt.message, got.Intent, tt.want)
			}
			if got.Parameters.TimeRange != TimeRangeThisMonth {
				t.Errorf("ClassifyAdviceIntent(%q).TimeRange = %q, want %q",
					tt.message, got.Parameters.TimeRange, TimeRangeThisMonth)
			}
		})
	}
}

func TestContainsAdviceKeywords(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{message: "tips hemat", want: true},
		{message: "gimana keuanganku", want: true},
		{message: "kasih insight dong", want: true},
		{message: "sebaiknya aku ngapain", want: true},
		{message: "berapa pengeluaran hari ini", want: false},
		{message: "saldo", want: false},
		{message: "makan siang 25rb", want: false},
	}

	for _, tt := range tests {
		got := ContainsAdviceKeywords(tt.message)
		if got != tt.want {
			t.Errorf("ContainsAdviceKeywords(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}
