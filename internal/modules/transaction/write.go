package transaction

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// ErrNoItems is returned when a save is attempted with nothing to save.
var ErrNoItems = errors.New("no transaction items provided")

// ManualInput is one transaction to write.
//
// CategoryIDOverride carries an already-resolved category, which is the normal
// case for the Telegram flow: the id was picked when the draft was built, so
// resolution must not run again at save time and silently choose differently.
type ManualInput struct {
	Amount             int64
	Description        string
	CategoryName       string
	CategoryIDOverride string
}

// BuildManualPayload assembles the Firestore document for one manual
// transaction, mirroring buildManualTransactionPayload (transactionService.ts:194).
//
// A map is used rather than domain.Transaction because the legacy document has
// no "id" field and omits the creator fields entirely when unknown. Marshalling
// the struct would introduce an empty id on every Telegram-created row and make
// these documents differ from the ones the web app writes.
func BuildManualPayload(amount int64, categoryID, description string, creatorUserID, creatorName string) map[string]interface{} {
	payload := map[string]interface{}{
		"amount":      amount,
		"categoryId":  categoryID,
		"description": description,
		"date":        datetime.TodayString(),
		"createdAt":   time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"source":      string(domain.TransactionSourceTelegram),
	}
	if creatorUserID != "" {
		payload["createdByUserId"] = creatorUserID
		payload["createdByName"] = creatorName
	}
	return payload
}

// ResolveManualCategoryID picks the category id to write, mirroring
// resolveManualTransactionCategoryId (transactionService.ts:172): an explicit
// override wins, then an exact case-insensitive name match, then the fallback.
func ResolveManualCategoryID(categoryName, override string, categories []domain.Category) (string, error) {
	if override != "" {
		return override, nil
	}
	if len(categories) == 0 {
		return "", ErrNoCategories
	}

	normalized := strings.ToLower(categoryName)
	for _, cat := range categories {
		if strings.ToLower(cat.Name) == normalized {
			return cat.ID, nil
		}
	}

	fallback, err := FallbackCategory(categories, domain.TransactionTypeExpense)
	if err != nil {
		return "", err
	}
	return fallback.ID, nil
}

// CreateManualBatch writes several transactions in one atomic batch and returns
// their document ids, porting createManualTransactionsBatch
// (transactionService.ts:408).
//
// Categories are fetched only when at least one item lacks an override, which is
// the legacy behavior and keeps the common Telegram path to a single write.
func CreateManualBatch(
	ctx context.Context,
	db *firestore.Client,
	svc *account.Service,
	repo *account.Repository,
	ac *account.Context,
	userID string,
	items []ManualInput,
) ([]string, error) {
	if len(items) == 0 {
		return nil, ErrNoItems
	}

	var categories []domain.Category
	for _, item := range items {
		if item.CategoryIDOverride == "" {
			var err error
			categories, err = repo.GetUserCategories(ctx, ac, false)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	creatorName := repo.GetCreatorName(ctx, userID)
	collection := svc.GetTransactionsCollection(ac)

	type pending struct {
		ref     *firestore.DocumentRef
		payload map[string]interface{}
	}

	writes := make([]pending, 0, len(items))
	ids := make([]string, 0, len(items))

	for _, item := range items {
		categoryID, err := ResolveManualCategoryID(item.CategoryName, item.CategoryIDOverride, categories)
		if err != nil {
			return nil, err
		}

		ref := collection.NewDoc()
		writes = append(writes, pending{
			ref:     ref,
			payload: BuildManualPayload(item.Amount, categoryID, item.Description, userID, creatorName),
		})
		ids = append(ids, ref.ID)
	}

	// All items commit together or none do. A multi-item message like
	// "kopi 18rb, parkir 5rb" is confirmed with a single "berhasil ditambahkan"
	// listing every row, so a partial write would make that message a lie.
	err := db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for _, w := range writes {
			if err := tx.Create(w.ref, w.payload); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}
