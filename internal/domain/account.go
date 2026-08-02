// Package domain defines Go structs mirroring the Firestore document schemas
// used by the DompetCerdas frontend and old backend. Field names use both
// `firestore` and `json` struct tags to ensure correct serialization in both
// directions. These structs are data-only — business logic lives in modules.
package domain

// AccountRole is the role a user holds within a financial account.
type AccountRole string

const (
	// AccountRoleOwner indicates the user owns the account.
	AccountRoleOwner AccountRole = "OWNER"
	// AccountRoleMember indicates the user is a collaborator.
	AccountRoleMember AccountRole = "MEMBER"
)

// FinancialAccount represents a user's financial workspace.
// Firestore path: users/{userId}/accounts/{accountId}
type FinancialAccount struct {
	ID              string      `firestore:"id"              json:"id"`
	Name            string      `firestore:"name"            json:"name"`
	Role            AccountRole `firestore:"role"            json:"role"`
	OwnerUserID     string      `firestore:"ownerUserId,omitempty"     json:"ownerUserId,omitempty"`
	SharedAccountID string      `firestore:"sharedAccountId,omitempty" json:"sharedAccountId,omitempty"`
	CreatedAt       string      `firestore:"createdAt"       json:"createdAt"`
	UpdatedAt       string      `firestore:"updatedAt"       json:"updatedAt"`
}

// SharedAccount is a collaborative workspace that multiple users can access.
// Firestore path: sharedAccounts/{sharedAccountId}
type SharedAccount struct {
	ID                  string `firestore:"id"                              json:"id"`
	Name                string `firestore:"name"                            json:"name"`
	OwnerUserID         string `firestore:"ownerUserId"                     json:"ownerUserId"`
	SourceAccountID     string `firestore:"sourceAccountId,omitempty"       json:"sourceAccountId,omitempty"`
	InviteCode          string `firestore:"inviteCode,omitempty"            json:"inviteCode,omitempty"`
	InviteCodeUpdatedAt string `firestore:"inviteCodeUpdatedAt,omitempty"   json:"inviteCodeUpdatedAt,omitempty"`
	InviteCodeExpiresAt string `firestore:"inviteCodeExpiresAt,omitempty"   json:"inviteCodeExpiresAt,omitempty"`
	CreatedAt           string `firestore:"createdAt"                       json:"createdAt"`
	UpdatedAt           string `firestore:"updatedAt"                       json:"updatedAt"`
}

// SharedAccountMember represents a user's membership in a shared account.
// Firestore path: sharedAccounts/{sharedAccountId}/members/{memberId}
type SharedAccountMember struct {
	ID          string      `firestore:"id"                       json:"id"`
	UserID      string      `firestore:"userId"                   json:"userId"`
	Role        AccountRole `firestore:"role"                     json:"role"`
	Email       string      `firestore:"email,omitempty"          json:"email,omitempty"`
	DisplayName string      `firestore:"displayName,omitempty"    json:"displayName,omitempty"`
	JoinedAt    string      `firestore:"joinedAt"                 json:"joinedAt"`
	UpdatedAt   string      `firestore:"updatedAt"                json:"updatedAt"`
}

// AccountContext resolves Firestore collection paths at runtime.
// The old codebase has three path variants depending on account age and type:
//   - Legacy:  users/{uid}/transactions  (old accounts before multi-account)
//   - Private: users/{uid}/accounts/{aid}/transactions
//   - Shared:  sharedAccounts/{sid}/transactions
type AccountContext struct {
	// UserID is the Firebase Auth UID.
	UserID string
	// AccountID is the financial account doc ID.
	AccountID string
	// SharedAccountID, if non-empty, indicates a shared workspace.
	SharedAccountID string
	// UsesLegacyPaths is true for old accounts that store data directly
	// under users/{uid}/ without an accounts/{aid}/ segment.
	UsesLegacyPaths bool
}

// CollectionPath returns the Firestore collection path for the given
// collection name (e.g. "transactions", "categories", "plans", "budgets",
// "debts", "routine_expenses", "routine_expense_records").
//
// The three variants:
//   - Legacy:  users/{uid}/{collection}
//   - Private: users/{uid}/accounts/{aid}/{collection}
//   - Shared:  sharedAccounts/{sid}/{collection}
func (ac AccountContext) CollectionPath(collection string) string {
	if ac.UsesLegacyPaths {
		return "users/" + ac.UserID + "/" + collection
	}
	if ac.SharedAccountID != "" {
		return "sharedAccounts/" + ac.SharedAccountID + "/" + collection
	}
	return "users/" + ac.UserID + "/accounts/" + ac.AccountID + "/" + collection
}
