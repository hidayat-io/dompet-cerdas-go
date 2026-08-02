package telegram

import (
	"encoding/json"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

// These tests assert byte-identical output against replies captured from the
// running legacy backend. A diff here means a user would see a different message
// than before the migration, which the parity contract does not allow without an
// ADR. See docs/PARITY_CONTRACT.md.

func TestParity_FormatTimeRange(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Spec   string `json:"spec"`
			Output string `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_time_range.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Spec, func(t *testing.T) {
			if got := FormatTimeRange(tc.Spec); got != tc.Output {
				t.Errorf("FormatTimeRange(%q) = %q, want %q", tc.Spec, got, tc.Output)
			}
		})
	}
}

func TestParity_FormatTransactionDetails(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label     string               `json:"label"`
			Details   []transaction.Detail `json:"details"`
			TimeRange string               `json:"timeRange"`
			Notice    string               `json:"notice"`
			Output    string               `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_transaction_details.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			got := FormatTransactionDetails(tc.Details, tc.TimeRange, tc.Notice)
			if got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}

func TestParity_FormatCategoryBreakdown(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label      string                          `json:"label"`
			Categories []transaction.CategoryBreakdown `json:"categories"`
			TimeRange  string                          `json:"timeRange"`
			Output     string                          `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_category_breakdown.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			got := FormatCategoryBreakdown(tc.Categories, tc.TimeRange)
			if got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}

func TestParity_FormatCategoryList(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label      string            `json:"label"`
			Categories []domain.Category `json:"categories"`
			Output     string            `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_category_list.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			got := FormatCategoryList(tc.Categories)
			if got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}

func TestParity_StaticReplies(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Fn     string          `json:"fn"`
			Input  json.RawMessage `json:"input"`
			Output string          `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_static_replies.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	str := func(t *testing.T, raw json.RawMessage) string {
		t.Helper()
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatalf("input is not a string: %v", err)
		}
		return s
	}

	covered := make(map[string]bool)

	for _, tc := range fixture.Cases {
		covered[tc.Fn] = true

		t.Run(tc.Fn, func(t *testing.T) {
			var got string
			switch tc.Fn {
			case "formatUnknownIntent":
				got = FormatUnknownIntent()
			case "formatClarification":
				got = FormatClarification(str(t, tc.Input))
			case "formatFinancialAdvice":
				got = FormatFinancialAdvice(str(t, tc.Input))
			case "formatSavingsStrategy":
				got = FormatSavingsStrategy(str(t, tc.Input))
			case "formatExpenseAnalysis":
				got = FormatExpenseAnalysis(str(t, tc.Input))
			case "withAccountHeader":
				var args []string
				if err := json.Unmarshal(tc.Input, &args); err != nil {
					t.Fatalf("input is not a string array: %v", err)
				}
				if len(args) != 2 {
					t.Fatalf("expected 2 arguments, got %d", len(args))
				}
				got = WithAccountHeader(args[0], args[1])
			default:
				t.Fatalf("fixture references unknown function %q — port it or drop the case", tc.Fn)
			}

			if got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}

	// Guards against a fixture that silently stops covering a formatter.
	for _, fn := range []string{
		"formatUnknownIntent",
		"formatClarification",
		"formatFinancialAdvice",
		"formatSavingsStrategy",
		"formatExpenseAnalysis",
		"withAccountHeader",
	} {
		if !covered[fn] {
			t.Errorf("fixture no longer covers %s", fn)
		}
	}
}

func TestParity_FormatTransactionDraftPreview(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label  string      `json:"label"`
			Items  []DraftItem `json:"items"`
			UsedAI bool        `json:"usedAI"`
			Output string      `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_draft_preview.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			if got := FormatTransactionDraftPreview(tc.Items, tc.UsedAI); got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}

func TestParity_FormatTransactionBatchAdded(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label  string      `json:"label"`
			Items  []DraftItem `json:"items"`
			Output string      `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_batch_added.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			if got := FormatTransactionBatchAdded(tc.Items); got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}

func TestParity_FormatAutoSavedTransaction(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Label        string `json:"label"`
			Amount       int64  `json:"amount"`
			Description  string `json:"description"`
			CategoryName string `json:"categoryName"`
			Output       string `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, "telegram_auto_saved.json", &fixture)

	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Label, func(t *testing.T) {
			got := FormatAutoSavedTransaction(tc.Amount, tc.Description, tc.CategoryName)
			if got != tc.Output {
				t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.Output)
			}
		})
	}
}
