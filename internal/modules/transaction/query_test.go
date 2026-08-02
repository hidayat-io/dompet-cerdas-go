package transaction

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

func TestSortDetails_ByDate(t *testing.T) {
	details := []Detail{
		{ID: "a", Date: "2026-07-01", CreatedAt: "2026-07-01T10:00:00Z", Amount: 5000},
		{ID: "b", Date: "2026-07-03", CreatedAt: "2026-07-03T08:00:00Z", Amount: 1000},
		{ID: "c", Date: "2026-07-03", CreatedAt: "2026-07-03T20:00:00Z", Amount: 2000},
		{ID: "d", Date: "2026-07-02", CreatedAt: "2026-07-02T12:00:00Z", Amount: 9000},
	}

	SortDetails(details, SortByDate)

	want := []string{"c", "b", "d", "a"}
	for i, id := range want {
		if details[i].ID != id {
			t.Errorf("position %d = %q, want %q (full order: %v)", i, details[i].ID, id, ids(details))
		}
	}
}

func TestSortDetails_ByAmount(t *testing.T) {
	details := []Detail{
		{ID: "a", Date: "2026-07-01", Amount: 5000},
		{ID: "b", Date: "2026-07-03", Amount: 1000},
		{ID: "c", Date: "2026-07-02", Amount: 9000},
	}

	SortDetails(details, SortByAmount)

	want := []string{"c", "a", "b"}
	for i, id := range want {
		if details[i].ID != id {
			t.Errorf("position %d = %q, want %q (full order: %v)", i, details[i].ID, id, ids(details))
		}
	}
}

// Amount mode has no tiebreaker in the legacy comparator, so equal amounts must
// retain input order rather than being reordered by date.
func TestSortDetails_ByAmount_EqualAmountsKeepInputOrder(t *testing.T) {
	details := []Detail{
		{ID: "first", Date: "2026-07-01", Amount: 5000},
		{ID: "second", Date: "2026-07-09", Amount: 5000},
		{ID: "third", Date: "2026-07-05", Amount: 5000},
	}

	SortDetails(details, SortByAmount)

	want := []string{"first", "second", "third"}
	for i, id := range want {
		if details[i].ID != id {
			t.Errorf("position %d = %q, want %q (full order: %v)", i, details[i].ID, id, ids(details))
		}
	}
}

func TestSortDetails_UnknownModeFallsBackToDate(t *testing.T) {
	details := []Detail{
		{ID: "a", Date: "2026-07-01", CreatedAt: "2026-07-01T00:00:00Z"},
		{ID: "b", Date: "2026-07-05", CreatedAt: "2026-07-05T00:00:00Z"},
	}

	SortDetails(details, "")

	if details[0].ID != "b" {
		t.Errorf("expected date-desc fallback, got %v", ids(details))
	}
}

func TestSortDetails_Empty(t *testing.T) {
	var details []Detail
	SortDetails(details, SortByAmount)
	SortDetails(details, SortByDate)
}

func TestClampTelegramLimit(t *testing.T) {
	tests := []struct {
		name        string
		requested   int
		wantLimit   int
		wantClamped bool
	}{
		{name: "under_cap", requested: 5, wantLimit: 5, wantClamped: false},
		{name: "at_cap", requested: 30, wantLimit: 30, wantClamped: false},
		{name: "over_cap", requested: 40, wantLimit: 30, wantClamped: true},
		{name: "zero_means_unlimited", requested: 0, wantLimit: 0, wantClamped: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, clamped := ClampTelegramLimit(tt.requested)
			if limit != tt.wantLimit || clamped != tt.wantClamped {
				t.Errorf("ClampTelegramLimit(%d) = (%d, %v), want (%d, %v)",
					tt.requested, limit, clamped, tt.wantLimit, tt.wantClamped)
			}
		})
	}
}

func TestDetailTypeDefaults(t *testing.T) {
	if domain.TransactionTypeExpense != "EXPENSE" {
		t.Errorf("TransactionTypeExpense = %q, want EXPENSE", domain.TransactionTypeExpense)
	}
	if domain.TransactionTypeIncome != "INCOME" {
		t.Errorf("TransactionTypeIncome = %q, want INCOME", domain.TransactionTypeIncome)
	}
}

func ids(details []Detail) []string {
	out := make([]string, len(details))
	for i, d := range details {
		out[i] = d.ID
	}
	return out
}
