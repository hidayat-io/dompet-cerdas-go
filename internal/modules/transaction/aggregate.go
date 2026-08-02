package transaction

import (
	"sort"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

// CategoryBreakdown is one row of the per-category expense summary, mirroring
// CategoryData in queryService.ts.
type CategoryBreakdown struct {
	Category   string  `json:"category"`
	Amount     int64   `json:"amount"`
	Percentage float64 `json:"percentage"`
	Count      int     `json:"count"`
	Icon       string  `json:"icon"`
}

// SumByType totals the transactions of one type.
//
// Rows whose category could not be resolved are skipped. getTotalExpenses and
// getTotalIncome (queryService.ts:180-189, 222-231) sum only transactions whose
// categoryId is in the expense/income category set, so a transaction pointing at
// a deleted category contributes to neither total. The detail listing still
// shows such a row as an "Other" expense, which is why the two disagree.
func SumByType(details []Detail, txType domain.TransactionType) (total int64, count int) {
	for _, d := range details {
		if !d.CategoryResolved || d.Type != txType {
			continue
		}
		total += d.Amount
		count++
	}
	return total, count
}

// NetBalance is income minus expenses, mirroring getBalance (queryService.ts:277-286).
// A row with an unresolved category matches neither branch there and is ignored here too.
func NetBalance(details []Detail) int64 {
	var balance int64
	for _, d := range details {
		if !d.CategoryResolved {
			continue
		}
		switch d.Type {
		case domain.TransactionTypeIncome:
			balance += d.Amount
		case domain.TransactionTypeExpense:
			balance -= d.Amount
		}
	}
	return balance
}

// BuildCategoryBreakdown groups expenses by category name and computes each
// share of the expense total, mirroring getCategoryBreakdown (queryService.ts:337-374).
//
// Grouping is by name, not id, so two categories that share a name merge into
// one row and the first row's icon wins — the legacy behavior. Percentages are
// a share of the expenses counted here, not of all transactions.
//
// Ties on amount keep input order. The legacy sort is stable too, but its input
// is an unordered Firestore snapshot, so the relative order of equal-amount
// categories was never actually defined; it only becomes visible when a caller
// truncates to a top-N.
func BuildCategoryBreakdown(details []Detail) []CategoryBreakdown {
	type bucket struct {
		amount int64
		count  int
		icon   string
	}

	order := make([]string, 0, len(details))
	buckets := make(map[string]*bucket, len(details))
	var totalAmount int64

	for _, d := range details {
		if !d.CategoryResolved || d.Type != domain.TransactionTypeExpense {
			continue
		}

		b, ok := buckets[d.Category]
		if !ok {
			b = &bucket{icon: d.Icon}
			buckets[d.Category] = b
			order = append(order, d.Category)
		}

		b.amount += d.Amount
		b.count++
		totalAmount += d.Amount
	}

	breakdown := make([]CategoryBreakdown, 0, len(order))
	for _, name := range order {
		b := buckets[name]
		var percentage float64
		if totalAmount > 0 {
			percentage = float64(b.amount) / float64(totalAmount) * 100
		}
		breakdown = append(breakdown, CategoryBreakdown{
			Category:   name,
			Amount:     b.amount,
			Percentage: percentage,
			Count:      b.count,
			Icon:       b.icon,
		})
	}

	sort.SliceStable(breakdown, func(i, j int) bool {
		return breakdown[i].Amount > breakdown[j].Amount
	})

	return breakdown
}
