package account

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

var collectionsToCopy = []string{
	"categories",
	"transactions",
	"plans",
	"budgets",
	"simulations",
	"debts",
	"routine_expenses",
	"routine_expense_records",
}

// StartSharedAccountMigration implements ADR-007: Resumable copy-then-soft-cutover.
func (s *Service) StartSharedAccountMigration(ctx context.Context, userID, accountID, accountName string) (*domain.SharedAccount, error) {
	sharedAccRef := s.db.Collection("sharedAccounts").NewDoc()
	jobRef := s.db.Collection("migration_jobs").NewDoc()
	userAccRef := s.db.Collection("users").Doc(userID).Collection("accounts").Doc(accountID)

	now := time.Now()
	nowStr := now.UTC().Format("2006-01-02T15:04:05.999Z")

	sharedAcc := domain.SharedAccount{
		Name:            accountName,
		OwnerUserID:     userID,
		SourceAccountID: accountID,
		CreatedAt:       nowStr,
		UpdatedAt:       nowStr,
	}

	member := domain.SharedAccountMember{
		UserID:    userID,
		Role:      "OWNER",
		JoinedAt:  nowStr,
		UpdatedAt: nowStr,
	}

	job := domain.MigrationJob{
		UserID:            userID,
		AccountID:         accountID,
		SharedAccountID:   sharedAccRef.ID,
		Phase:             domain.MigrationPhaseCreated,
		CopiedCollections: make(map[string]bool),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	err := s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Set(sharedAccRef, sharedAcc); err != nil {
			return err
		}
		if err := tx.Set(sharedAccRef.Collection("members").Doc(userID), member); err != nil {
			return err
		}
		if err := tx.Set(jobRef, job); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. Trigger async copy process
	go s.resumeMigration(context.Background(), jobRef.ID, job, userAccRef, sharedAccRef)

	sharedAcc.ID = sharedAccRef.ID
	return &sharedAcc, nil
}

// resumeMigration handles Phase 2 (Copying) and Phase 3 (Cutover)
func (s *Service) resumeMigration(ctx context.Context, jobID string, job domain.MigrationJob, sourceAccRef, targetAccRef *firestore.DocumentRef) {
	jobRef := s.db.Collection("migration_jobs").Doc(jobID)

	// Phase 2: COPYING
	s.updateJobPhase(ctx, jobRef, domain.MigrationPhaseCopying)

	for _, collName := range collectionsToCopy {
		if job.CopiedCollections[collName] {
			continue // Skip already copied
		}

		sourceCol := sourceAccRef.Collection(collName)
		targetCol := targetAccRef.Collection(collName)

		iter := sourceCol.Documents(ctx)
		snaps, err := iter.GetAll()
		if err != nil {
			continue
		}

		if len(snaps) > 0 {
			batch := s.db.Batch()
			count := 0
			for _, snap := range snaps {
				batch.Set(targetCol.Doc(snap.Ref.ID), snap.Data())
				count++
				if count == 400 {
					batch.Commit(ctx)
					batch = s.db.Batch()
					count = 0
				}
			}
			if count > 0 {
				batch.Commit(ctx)
			}
		}

		job.CopiedCollections[collName] = true
		s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			return tx.Update(jobRef, []firestore.Update{
				{Path: "copiedCollections." + collName, Value: true},
				{Path: "updatedAt", Value: time.Now()},
			})
		})
	}

	s.updateJobPhase(ctx, jobRef, domain.MigrationPhaseCopyDone)

	s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.999Z")
		tx.Update(sourceAccRef, []firestore.Update{
			{Path: "sharedAccountId", Value: targetAccRef.ID},
			{Path: "migrated", Value: true},
			{Path: "updatedAt", Value: nowStr},
		})

		userRef := s.db.Collection("users").Doc(job.UserID)
		tx.Update(userRef, []firestore.Update{
			{Path: "activeAccountId", Value: job.AccountID},
			{Path: "updatedAt", Value: nowStr},
		})

		return nil
	})

	s.updateJobPhase(ctx, jobRef, domain.MigrationPhaseCutoverDone)
}

func (s *Service) updateJobPhase(ctx context.Context, jobRef *firestore.DocumentRef, phase domain.MigrationPhase) {
	s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		return tx.Update(jobRef, []firestore.Update{
			{Path: "phase", Value: phase},
			{Path: "updatedAt", Value: time.Now()},
		})
	})
}
