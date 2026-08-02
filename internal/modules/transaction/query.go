package transaction

import (
	"context"
	"sort"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
)

// TelegramMaxResults caps how many transactions a Telegram reply may list.
// The legacy bot clamps a larger request down to this value and appends a notice
// (bot/index.ts:1333-1335).
const TelegramMaxResults = 30

// SortByAmount and SortByDate are the accepted sort modes.
const (
	SortByAmount = "amount"
	SortByDate   = "date"
)

// QueryParams describes a transaction query.
//
// StartDate and EndDate are inclusive "YYYY-MM-DD" bounds and are the only
// constraints pushed to Firestore. CategoryName filters on the resolved category
// name, not the ID, matching the legacy behavior. Limit of 0 means unlimited.
type QueryParams struct {
	StartDate    string
	EndDate      string
	CategoryName string
	TypeFilter   domain.TransactionType
	SortBy       string
	Limit        int
}

// Detail is a transaction joined with its category metadata, mirroring
// TransactionDetail in queryService.ts.
//
// CategoryResolved records whether the transaction's categoryId actually matched
// a category document. The detail listing renders unmatched rows as "Other"
// expenses, but every aggregate in queryService.ts skips them instead — see
// SumByType. Without this flag that distinction cannot be recovered downstream.
type Detail struct {
	ID               string                 `json:"id"`
	Description      string                 `json:"description"`
	Amount           int64                  `json:"amount"`
	Category         string                 `json:"category"`
	Date             string                 `json:"date"`
	CreatedAt        string                 `json:"createdAt"`
	Icon             string                 `json:"icon"`
	Type             domain.TransactionType `json:"type"`
	CategoryResolved bool                   `json:"-"`
}

// Defaults applied when a transaction's category cannot be resolved, matching
// the legacy fallbacks.
const (
	fallbackCategoryName = "Other"
	fallbackCategoryIcon = "Package"
	fallbackDescription  = "-"
)

// QueryTransactions fetches transactions in a date range and applies category
// join, type filter, sort, and limit in memory.
//
// Only the date range is sent to Firestore. Type and category filtering need
// category metadata that lives in a separate collection, and sorting in the
// query would require composite indexes, so both are done in memory. This is
// the legacy strategy and it scales with the size of the date range, not the
// account.
func QueryTransactions(
	ctx context.Context,
	svc *account.Service,
	repo *account.Repository,
	ac *account.Context,
	params QueryParams,
) ([]Detail, error) {
	categories, err := repo.GetUserCategories(ctx, ac, false)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]domain.Category, len(categories))
	for _, cat := range categories {
		byID[cat.ID] = cat
	}

	iter := svc.GetTransactionsCollection(ac).
		Where("date", ">=", params.StartDate).
		Where("date", "<=", params.EndDate).
		Documents(ctx)
	defer iter.Stop()

	snaps, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	details := make([]Detail, 0, len(snaps))
	for _, snap := range snaps {
		var tx domain.Transaction
		if err := snap.DataTo(&tx); err != nil {
			continue
		}

		categoryName := fallbackCategoryName
		categoryIcon := fallbackCategoryIcon
		txType := domain.TransactionTypeExpense

		cat, resolved := byID[tx.CategoryID]
		if resolved {
			if cat.Name != "" {
				categoryName = cat.Name
			}
			if cat.Icon != "" {
				categoryIcon = cat.Icon
			}
			if cat.Type != "" {
				txType = cat.Type
			}
		}

		if params.CategoryName != "" && categoryName != params.CategoryName {
			continue
		}
		if params.TypeFilter != "" && txType != params.TypeFilter {
			continue
		}

		description := tx.Description
		if description == "" {
			description = fallbackDescription
		}

		// The legacy code falls back to the date when createdAt is absent, so
		// that the secondary sort key is never empty.
		createdAt := tx.CreatedAt
		if createdAt == "" {
			createdAt = tx.Date
		}

		details = append(details, Detail{
			ID:               snap.Ref.ID,
			Description:      description,
			Amount:           tx.Amount,
			Category:         categoryName,
			Date:             tx.Date,
			CreatedAt:        createdAt,
			Icon:             categoryIcon,
			Type:             txType,
			CategoryResolved: resolved,
		})
	}

	SortDetails(details, params.SortBy)

	if params.Limit > 0 && len(details) > params.Limit {
		details = details[:params.Limit]
	}

	return details, nil
}

// SortDetails orders details in place.
//
// Amount mode sorts by amount descending only, with no tiebreaker, matching the
// legacy single-key comparator. Any other mode sorts by date descending then
// createdAt descending. sort.SliceStable is used so that equal keys keep their
// relative order rather than being shuffled arbitrarily.
func SortDetails(details []Detail, sortBy string) {
	if sortBy == SortByAmount {
		sort.SliceStable(details, func(i, j int) bool {
			return details[i].Amount > details[j].Amount
		})
		return
	}

	sort.SliceStable(details, func(i, j int) bool {
		if details[i].Date != details[j].Date {
			return details[i].Date > details[j].Date
		}
		return details[i].CreatedAt > details[j].CreatedAt
	})
}

// ClampTelegramLimit applies the Telegram reply cap. It reports whether the
// requested limit was reduced, so the caller can surface the same notice the
// legacy bot shows.
func ClampTelegramLimit(requested int) (limit int, clamped bool) {
	if requested > TelegramMaxResults {
		return TelegramMaxResults, true
	}
	return requested, false
}
