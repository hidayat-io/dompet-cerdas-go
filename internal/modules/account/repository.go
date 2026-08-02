package account

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

// CategoryCacheTTL matches CATEGORY_CACHE_TTL_MS in transactionService.ts.
//
// On Cloud Functions this cache was effectively reset on every cold start, so a
// stale entry rarely survived long. A long-lived Go process holds it for the
// full window, which means a category renamed in the web app can stay stale in
// the bot for up to 24h. The refresh-cache endpoint exists to force
// invalidation; see the open question in docs/DECISIONS.md.
const CategoryCacheTTL = 24 * time.Hour

// CreatorNameFallback is used when a user document has no display name, matching
// the legacy fallback string.
const CreatorNameFallback = "Pengguna"

type categoryCacheEntry struct {
	expiresAt  time.Time
	categories []domain.Category
}

// Repository provides cached reads of account-scoped reference data.
type Repository struct {
	db               *firestore.Client
	service          *Service
	categoryCache    sync.Map
	creatorNameCache sync.Map
}

// NewRepository constructs a repository bound to an account service, which owns
// path resolution.
func NewRepository(db *firestore.Client, service *Service) *Repository {
	return &Repository{db: db, service: service}
}

// GetCreatorName resolves a user's display name, caching the result.
//
// The legacy creatorNameCache has no TTL. That is preserved here: display names
// change rarely, and a stale name is cosmetic rather than financial.
func (r *Repository) GetCreatorName(ctx context.Context, userID string) string {
	if val, ok := r.creatorNameCache.Load(userID); ok {
		if name, ok := val.(string); ok {
			return name
		}
	}

	snap, err := r.db.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return CreatorNameFallback
	}

	var data struct {
		DisplayName string `firestore:"displayName"`
	}
	if err := snap.DataTo(&data); err != nil || data.DisplayName == "" {
		return CreatorNameFallback
	}

	r.creatorNameCache.Store(userID, data.DisplayName)
	return data.DisplayName
}

// InvalidateCreatorName drops a cached display name.
func (r *Repository) InvalidateCreatorName(userID string) {
	r.creatorNameCache.Delete(userID)
}

// GetUserCategories returns the categories for the given context, served from
// cache when a fresh entry exists.
//
// When the account has no category documents, the default set is returned but
// deliberately NOT cached, so that a genuinely empty account picks up its real
// categories as soon as they are created rather than serving defaults for 24h.
func (r *Repository) GetUserCategories(ctx context.Context, ac *Context, forceRefresh bool) ([]domain.Category, error) {
	key := ac.CacheKey()

	if !forceRefresh {
		if val, ok := r.categoryCache.Load(key); ok {
			if entry, ok := val.(categoryCacheEntry); ok && time.Now().Before(entry.expiresAt) {
				return entry.categories, nil
			}
		}
	}

	iter := r.service.GetCategoriesCollection(ac).Documents(ctx)
	defer iter.Stop()

	snaps, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	categories := make([]domain.Category, 0, len(snaps))
	for _, snap := range snaps {
		var cat domain.Category
		if err := snap.DataTo(&cat); err != nil {
			continue
		}
		cat.ID = snap.Ref.ID
		categories = append(categories, cat)
	}

	if len(categories) == 0 {
		return DefaultCategories(), nil
	}

	r.categoryCache.Store(key, categoryCacheEntry{
		expiresAt:  time.Now().Add(CategoryCacheTTL),
		categories: categories,
	})

	return categories, nil
}

// InvalidateCategoryCache drops the cached categories for a context.
func (r *Repository) InvalidateCategoryCache(ac *Context) {
	r.categoryCache.Delete(ac.CacheKey())
}

// DefaultCategories returns the seed category set, mirroring
// DEFAULT_CATEGORY_DOCS in transactionService.ts and DEFAULT_SHARED_CATEGORIES
// in sharedAccountService.ts.
func DefaultCategories() []domain.Category {
	return []domain.Category{
		{ID: "c1", Name: "Gaji", Type: domain.TransactionTypeIncome, Icon: "Wallet", Color: "#10b981"},
		{ID: "c2", Name: "Bonus", Type: domain.TransactionTypeIncome, Icon: "Gift", Color: "#34d399"},
		{ID: "c3", Name: "Makanan", Type: domain.TransactionTypeExpense, Icon: "Utensils", Color: "#f87171"},
		{ID: "c4", Name: "Transport", Type: domain.TransactionTypeExpense, Icon: "Car", Color: "#60a5fa"},
		{ID: "c5", Name: "Belanja", Type: domain.TransactionTypeExpense, Icon: "ShoppingBag", Color: "#f472b6"},
		{ID: "c6", Name: "Tagihan", Type: domain.TransactionTypeExpense, Icon: "Zap", Color: "#fbbf24"},
	}
}
