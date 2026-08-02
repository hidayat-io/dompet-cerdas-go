package domain

import "time"

type MigrationPhase string

const (
	MigrationPhaseCreated     MigrationPhase = "CREATED"
	MigrationPhaseCopying     MigrationPhase = "COPYING"
	MigrationPhaseCopyDone    MigrationPhase = "COPY_DONE"
	MigrationPhaseCutoverDone MigrationPhase = "CUTOVER_DONE"
	MigrationPhaseCleanupDone MigrationPhase = "CLEANUP_DONE"
)

// MigrationJob tracks the progress of converting a private account to a shared workspace (ADR-007).
type MigrationJob struct {
	ID                string          `firestore:"-" json:"id"`
	UserID            string          `firestore:"userId" json:"user_id"`
	AccountID         string          `firestore:"accountId" json:"account_id"`
	SharedAccountID   string          `firestore:"sharedAccountId" json:"shared_account_id"`
	Phase             MigrationPhase  `firestore:"phase" json:"phase"`
	CopiedCollections map[string]bool `firestore:"copiedCollections" json:"copied_collections"`
	CreatedAt         time.Time       `firestore:"createdAt" json:"created_at"`
	UpdatedAt         time.Time       `firestore:"updatedAt" json:"updated_at"`
}
