package transaction

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

func expense(category string, amount int64) Detail {
	return Detail{
		Category:         category,
		Amount:           amount,
		Icon:             "Utensils",
		Type:             domain.TransactionTypeExpense,
		CategoryResolved: true,
	}
}

func income(category string, amount int64) Detail {
	return Detail{
		Category:         category,
		Amount:           amount,
		Icon:             "Wallet",
		Type:             domain.TransactionTypeIncome,
		CategoryResolved: true,
	}
}

// orphan is a transaction whose categoryId no longer matches a category
// document. QueryTransactions renders it as an "Other" expense.
func orphan(amount int64) Detail {
	return Detail{
		Category:         "Other",
		Amount:           amount,
		Icon:             "Package",
		Type:             domain.TransactionTypeExpense,
		CategoryResolved: false,
	}
}

func TestSumByType(t *testing.T) {
	details := []Detail{
		expense("Makanan", 25_000),
		expense("Transport", 50_000),
		income("Gaji", 5_000_000),
	}

	total, count := SumByType(details, domain.TransactionTypeExpense)
	if total != 75_000 || count != 2 {
		t.Errorf("expenses = (%d, %d), want (75000, 2)", total, count)
	}

	total, count = SumByType(details, domain.TransactionTypeIncome)
	if total != 5_000_000 || count != 1 {
		t.Errorf("income = (%d, %d), want (5000000, 1)", total, count)
	}
}

func TestSumByType_Empty(t *testing.T) {
	total, count := SumByType(nil, domain.TransactionTypeExpense)
	if total != 0 || count != 0 {
		t.Errorf("empty = (%d, %d), want (0, 0)", total, count)
	}
}

// A transaction pointing at a deleted category is listed as an expense but must
// not be summed, because getTotalExpenses filters on the expense category set.
func TestSumByType_SkipsUnresolvedCategory(t *testing.T) {
	details := []Detail{expense("Makanan", 25_000), orphan(999_000)}

	total, count := SumByType(details, domain.TransactionTypeExpense)
	if total != 25_000 || count != 1 {
		t.Errorf("expenses = (%d, %d), want (25000, 1) — orphan row must be skipped", total, count)
	}
}

func TestNetBalance(t *testing.T) {
	details := []Detail{
		income("Gaji", 5_000_000),
		expense("Makanan", 750_000),
		expense("Transport", 250_000),
	}

	if got := NetBalance(details); got != 4_000_000 {
		t.Errorf("NetBalance = %d, want 4000000", got)
	}
}

func TestNetBalance_NegativeWhenExpensesExceedIncome(t *testing.T) {
	details := []Detail{income("Gaji", 100_000), expense("Makanan", 250_000)}

	if got := NetBalance(details); got != -150_000 {
		t.Errorf("NetBalance = %d, want -150000", got)
	}
}

func TestNetBalance_SkipsUnresolvedCategory(t *testing.T) {
	details := []Detail{income("Gaji", 100_000), orphan(70_000)}

	if got := NetBalance(details); got != 100_000 {
		t.Errorf("NetBalance = %d, want 100000 — orphan row must not subtract", got)
	}
}

func TestBuildCategoryBreakdown(t *testing.T) {
	details := []Detail{
		expense("Transport", 300_000),
		expense("Makanan", 500_000),
		expense("Makanan", 250_000),
		income("Gaji", 9_000_000),
	}

	got := BuildCategoryBreakdown(details)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (income must be excluded)", len(got))
	}
	if got[0].Category != "Makanan" || got[0].Amount != 750_000 || got[0].Count != 2 {
		t.Errorf("row 0 = %+v, want Makanan/750000/2", got[0])
	}
	if got[1].Category != "Transport" || got[1].Amount != 300_000 {
		t.Errorf("row 1 = %+v, want Transport/300000", got[1])
	}

	// Percentages are a share of expenses only: 750k and 300k of 1.05M.
	if want := 750_000.0 / 1_050_000.0 * 100; got[0].Percentage != want {
		t.Errorf("row 0 percentage = %v, want %v", got[0].Percentage, want)
	}
}

func TestBuildCategoryBreakdown_Empty(t *testing.T) {
	if got := BuildCategoryBreakdown(nil); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// Percentage divides by the expense total, so an all-income input must not
// divide by zero.
func TestBuildCategoryBreakdown_IncomeOnlyHasNoRows(t *testing.T) {
	got := BuildCategoryBreakdown([]Detail{income("Gaji", 5_000_000)})
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestBuildCategoryBreakdown_MergesByNameAndKeepsFirstIcon(t *testing.T) {
	first := expense("Makanan", 100_000)
	first.Icon = "Utensils"
	second := expense("Makanan", 50_000)
	second.Icon = "Pizza"

	got := BuildCategoryBreakdown([]Detail{first, second})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 — same name must merge", len(got))
	}
	if got[0].Amount != 150_000 {
		t.Errorf("amount = %d, want 150000", got[0].Amount)
	}
	if got[0].Icon != "Utensils" {
		t.Errorf("icon = %q, want %q — the first row's icon wins", got[0].Icon, "Utensils")
	}
}

func TestBuildCategoryBreakdown_SkipsUnresolvedCategory(t *testing.T) {
	got := BuildCategoryBreakdown([]Detail{expense("Makanan", 100_000), orphan(900_000)})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 — orphan row must be excluded", len(got))
	}
	if got[0].Percentage != 100 {
		t.Errorf("percentage = %v, want 100 — orphan must not dilute the share", got[0].Percentage)
	}
}

func TestBuildCategoryBreakdown_TiesKeepInputOrder(t *testing.T) {
	got := BuildCategoryBreakdown([]Detail{
		expense("Bravo", 100_000),
		expense("Alpha", 100_000),
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Category != "Bravo" || got[1].Category != "Alpha" {
		t.Errorf("order = %s,%s, want Bravo,Alpha", got[0].Category, got[1].Category)
	}
}
