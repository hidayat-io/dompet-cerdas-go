package account

import (
	"context"
	"strings"

	"cloud.google.com/go/firestore"
)

// Collection names used across the three path variants.
const (
	CollectionCategories            = "categories"
	CollectionTransactions          = "transactions"
	CollectionPlans                 = "plans"
	CollectionBudgets               = "budgets"
	CollectionDebts                 = "debts"
	CollectionSimulations           = "simulations"
	CollectionRoutineExpenses       = "routine_expenses"
	CollectionRoutineExpenseRecords = "routine_expense_records"
)

// Context encapsulates which Firestore path variant applies to a request.
//
// The legacy backend resolves one of three shapes per request:
//
//	legacy:  users/{uid}/{collection}
//	private: users/{uid}/accounts/{aid}/{collection}
//	shared:  sharedAccounts/{sid}/{collection}
//
// Precedence is legacy, then shared, then private. Resolving the wrong shape
// reads or writes another user's financial data, so path construction is
// centralised here and never assembled ad hoc by callers.
type Context struct {
	UserID          string
	AccountID       string
	SharedAccountID string
	Role            string
	UsesLegacyPaths bool
}

// CollectionPath returns the fully-qualified Firestore path for a collection
// under this context.
//
// The legacy variant has no dedicated "plans" collection: plans live under
// "simulations" there. Callers should use PlansPath rather than passing
// CollectionPlans directly, so that remapping is applied.
func (ac *Context) CollectionPath(collection string) string {
	if ac.UsesLegacyPaths {
		return "users/" + ac.UserID + "/" + collection
	}
	if ac.SharedAccountID != "" {
		return "sharedAccounts/" + ac.SharedAccountID + "/" + collection
	}
	return "users/" + ac.UserID + "/accounts/" + ac.AccountID + "/" + collection
}

// CategoriesPath returns the categories collection path for this context.
func (ac *Context) CategoriesPath() string {
	return ac.CollectionPath(CollectionCategories)
}

// TransactionsPath returns the transactions collection path for this context.
func (ac *Context) TransactionsPath() string {
	return ac.CollectionPath(CollectionTransactions)
}

// PlansPath returns the plans collection path, remapping to "simulations" on
// the legacy variant where no "plans" collection exists.
func (ac *Context) PlansPath() string {
	if ac.UsesLegacyPaths {
		return ac.CollectionPath(CollectionSimulations)
	}
	return ac.CollectionPath(CollectionPlans)
}

// CacheKey identifies this context for per-account caching. Shared accounts key
// off the user's account stub ID rather than the shared workspace ID, matching
// the legacy cache key format `${userId}:${accountId|'legacy'}`.
func (ac *Context) CacheKey() string {
	if ac.UsesLegacyPaths {
		return ac.UserID + ":legacy"
	}
	return ac.UserID + ":" + ac.AccountID
}

// IsShared reports whether this context targets a shared workspace.
func (ac *Context) IsShared() bool {
	return !ac.UsesLegacyPaths && ac.SharedAccountID != ""
}

// IsOwner reports whether the user owns the shared workspace.
func (ac *Context) IsOwner() bool {
	return ac.Role == "OWNER"
}

// CanMutateRecord applies the shared-workspace permission model.
//
// Owners may mutate any record. Members may mutate only records they created.
// Records with an empty createdByUserId predate ownership tracking and fall
// back to owner-only.
//
// This is enforced server-side regardless of firestore.rules, which currently
// leaves shared transaction updates open to any member behind a
// "TODO: Re-enable after debugging" comment.
func (ac *Context) CanMutateRecord(recordCreatorUserID string) bool {
	if !ac.IsShared() {
		return true
	}
	if ac.IsOwner() {
		return true
	}
	if recordCreatorUserID == "" {
		return false
	}
	return recordCreatorUserID == ac.UserID
}

// Service resolves account contexts and Firestore collection references.
type Service struct {
	db *firestore.Client
}

// NewService constructs an account service backed by the given Firestore client.
func NewService(db *firestore.Client) *Service {
	return &Service{db: db}
}

// GetAccountContext resolves which path variant applies to the given user.
//
// preferredAccountID overrides the user's activeAccountId when non-empty. If
// neither resolves to an existing account document, the legacy variant is used,
// which is how pre-account-scoping users continue to work.
func (s *Service) GetAccountContext(ctx context.Context, userID, preferredAccountID string) (*Context, error) {
	userRef := s.db.Collection("users").Doc(userID)
	userSnap, err := userRef.Get(ctx)
	if err != nil {
		return &Context{UserID: userID, UsesLegacyPaths: true}, nil
	}

	var userData struct {
		ActiveAccountID string `firestore:"activeAccountId"`
	}
	if err := userSnap.DataTo(&userData); err != nil {
		return &Context{UserID: userID, UsesLegacyPaths: true}, nil
	}

	resolvedAccountID := preferredAccountID
	if resolvedAccountID == "" {
		resolvedAccountID = userData.ActiveAccountID
	}
	resolvedAccountID = strings.TrimSpace(resolvedAccountID)

	if resolvedAccountID != "" {
		accountSnap, err := userRef.Collection("accounts").Doc(resolvedAccountID).Get(ctx)
		if err == nil && accountSnap.Exists() {
			var accountData struct {
				SharedAccountID string `firestore:"sharedAccountId"`
				Role            string `firestore:"role"`
			}
			if err := accountSnap.DataTo(&accountData); err == nil {
				return &Context{
					UserID:          userID,
					AccountID:       resolvedAccountID,
					SharedAccountID: accountData.SharedAccountID,
					Role:            accountData.Role,
					UsesLegacyPaths: false,
				}, nil
			}
		}
	}

	return &Context{UserID: userID, UsesLegacyPaths: true}, nil
}

// GetCollection returns the Firestore collection reference for this context.
func (s *Service) GetCollection(ac *Context, collection string) *firestore.CollectionRef {
	return s.db.Collection(ac.CollectionPath(collection))
}

// GetCategoriesCollection returns the categories collection for this context.
func (s *Service) GetCategoriesCollection(ac *Context) *firestore.CollectionRef {
	return s.db.Collection(ac.CategoriesPath())
}

// GetTransactionsCollection returns the transactions collection for this context.
func (s *Service) GetTransactionsCollection(ac *Context) *firestore.CollectionRef {
	return s.db.Collection(ac.TransactionsPath())
}

// GetPlansCollection returns the plans collection for this context.
func (s *Service) GetPlansCollection(ac *Context) *firestore.CollectionRef {
	return s.db.Collection(ac.PlansPath())
}

// UserAccount is one entry from a user's account list.
type UserAccount struct {
	ID              string
	Name            string
	SharedAccountID string
	Role            string
}

// ListUserAccounts returns the user's accounts in creation order, porting
// getUserAccounts (accountService.ts).
//
// A missing name falls back to "Akun", matching the legacy default, so callers
// always have something to display.
func (s *Service) ListUserAccounts(ctx context.Context, userID string) ([]UserAccount, error) {
	snaps, err := s.db.Collection("users").Doc(userID).Collection("accounts").
		OrderBy("createdAt", firestore.Asc).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	accounts := make([]UserAccount, 0, len(snaps))
	for _, snap := range snaps {
		var data struct {
			Name            string `firestore:"name"`
			SharedAccountID string `firestore:"sharedAccountId"`
			Role            string `firestore:"role"`
		}
		if err := snap.DataTo(&data); err != nil {
			continue
		}
		if data.Name == "" {
			data.Name = "Akun"
		}
		accounts = append(accounts, UserAccount{
			ID:              snap.Ref.ID,
			Name:            data.Name,
			SharedAccountID: data.SharedAccountID,
			Role:            data.Role,
		})
	}
	return accounts, nil
}

// ContextForAccount builds a Context for one listed account without re-reading
// the user document, for callers that already enumerated the accounts.
func ContextForAccount(userID string, acc UserAccount) *Context {
	return &Context{
		UserID:          userID,
		AccountID:       acc.ID,
		SharedAccountID: acc.SharedAccountID,
		Role:            acc.Role,
	}
}

// LegacyContext is the pre-account-scoping path variant for a user.
func LegacyContext(userID string) *Context {
	return &Context{UserID: userID, UsesLegacyPaths: true}
}
