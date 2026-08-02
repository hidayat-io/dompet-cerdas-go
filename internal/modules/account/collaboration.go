package account

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	sharedcrypto "github.com/mthidayat/dompet-cerdas-go/internal/shared/crypto"
)

// Domain errors mapped to HTTP by handlers.
var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrNotSharedAccount    = errors.New("not a shared account")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrInviteNotFound      = errors.New("invite code not found")
	ErrInviteExpired       = errors.New("invite code expired")
	ErrInviteExhausted     = errors.New("failed to generate invite code")
	ErrLastAccountRequired = errors.New("at least one account must remain")
	ErrMembersStillPresent = errors.New("shared account still has other members")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrAlreadyShared       = errors.New("account already shared")
)

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func buildMember(userID, role, email, displayName, now string) domain.SharedAccountMember {
	return domain.SharedAccountMember{
		UserID:      userID,
		Role:        domain.AccountRole(role),
		Email:       email,
		DisplayName: displayName,
		JoinedAt:    now,
		UpdatedAt:   now,
	}
}

// CreateSharedAccount creates a brand-new shared workspace for the user.
func (s *Service) CreateSharedAccount(ctx context.Context, userID, name, email, displayName string) (map[string]interface{}, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidArgument
	}

	now := nowISO()
	userRef := s.db.Collection("users").Doc(userID)
	userAccountRef := userRef.Collection("accounts").NewDoc()
	sharedAccountRef := s.db.Collection("sharedAccounts").NewDoc()
	memberRef := sharedAccountRef.Collection("members").Doc(userID)

	batch := s.db.Batch()
	batch.Set(sharedAccountRef, map[string]interface{}{
		"name":                name,
		"ownerUserId":         userID,
		"inviteCode":          nil,
		"inviteCodeUpdatedAt": nil,
		"createdAt":           now,
		"updatedAt":           now,
	})
	batch.Set(memberRef, buildMember(userID, "OWNER", email, displayName, now))
	batch.Set(userAccountRef, map[string]interface{}{
		"name":            name,
		"role":            "OWNER",
		"ownerUserId":     userID,
		"sharedAccountId": sharedAccountRef.ID,
		"createdAt":       now,
		"updatedAt":       now,
	})
	for _, cat := range DefaultCategories() {
		batch.Set(sharedAccountRef.Collection("categories").Doc(cat.ID), cat)
	}
	batch.Set(userRef, map[string]interface{}{
		"activeAccountId": userAccountRef.ID,
		"updatedAt":       now,
	}, firestore.MergeAll)

	if _, err := batch.Commit(ctx); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":         true,
		"accountId":       userAccountRef.ID,
		"sharedAccountId": sharedAccountRef.ID,
		"name":            name,
	}, nil
}

// CreateSharedInviteCode generates a unique 8-char invite code valid for 7 days.
func (s *Service) CreateSharedInviteCode(ctx context.Context, userID, accountID string) (map[string]interface{}, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrInvalidArgument
	}

	userAccountRef := s.db.Collection("users").Doc(userID).Collection("accounts").Doc(accountID)
	snap, err := userAccountRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	var accountData struct {
		SharedAccountID string `firestore:"sharedAccountId"`
		Role            string `firestore:"role"`
	}
	if err := snap.DataTo(&accountData); err != nil {
		return nil, err
	}
	if accountData.SharedAccountID == "" {
		return nil, ErrNotSharedAccount
	}
	if accountData.Role != "OWNER" {
		return nil, ErrPermissionDenied
	}

	var code string
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := sharedcrypto.InviteCode()
		if err != nil {
			return nil, err
		}
		existing, err := s.db.Collection("sharedAccounts").
			Where("inviteCode", "==", candidate).
			Limit(1).
			Documents(ctx).
			GetAll()
		if err != nil {
			return nil, err
		}
		if len(existing) == 0 {
			code = candidate
			break
		}
	}
	if code == "" {
		return nil, ErrInviteExhausted
	}

	now := nowISO()
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
	_, err = s.db.Collection("sharedAccounts").Doc(accountData.SharedAccountID).Set(ctx, map[string]interface{}{
		"inviteCode":          code,
		"inviteCodeUpdatedAt": now,
		"inviteCodeExpiresAt": expiresAt,
		"updatedAt":           now,
	}, firestore.MergeAll)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":   true,
		"code":      code,
		"expiresAt": expiresAt,
	}, nil
}

// JoinSharedAccountByCode joins a shared workspace via invite code.
func (s *Service) JoinSharedAccountByCode(ctx context.Context, userID, rawCode, email, displayName string) (map[string]interface{}, error) {
	code := normalizeInviteCode(rawCode)
	if code == "" {
		return nil, ErrInvalidArgument
	}

	snaps, err := s.db.Collection("sharedAccounts").
		Where("inviteCode", "==", code).
		Limit(1).
		Documents(ctx).
		GetAll()
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, ErrInviteNotFound
	}

	sharedDoc := snaps[0]
	var sharedData struct {
		Name                string `firestore:"name"`
		OwnerUserID         string `firestore:"ownerUserId"`
		InviteCodeExpiresAt string `firestore:"inviteCodeExpiresAt"`
	}
	if err := sharedDoc.DataTo(&sharedData); err != nil {
		return nil, err
	}

	if sharedData.InviteCodeExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339Nano, sharedData.InviteCodeExpiresAt)
		if err == nil && time.Now().After(exp) {
			// also try RFC3339
			exp2, err2 := time.Parse(time.RFC3339, sharedData.InviteCodeExpiresAt)
			if err2 == nil && time.Now().After(exp2) {
				return nil, ErrInviteExpired
			}
			if err2 != nil && time.Now().After(exp) {
				return nil, ErrInviteExpired
			}
		} else if err != nil {
			if exp2, err2 := time.Parse(time.RFC3339, sharedData.InviteCodeExpiresAt); err2 == nil && time.Now().After(exp2) {
				return nil, ErrInviteExpired
			}
		}
	}

	userRef := s.db.Collection("users").Doc(userID)
	existingStub, err := userRef.Collection("accounts").
		Where("sharedAccountId", "==", sharedDoc.Ref.ID).
		Limit(1).
		Documents(ctx).
		GetAll()
	if err != nil {
		return nil, err
	}

	var accountRef *firestore.DocumentRef
	existingCreatedAt := nowISO()
	if len(existingStub) == 0 {
		accountRef = userRef.Collection("accounts").NewDoc()
	} else {
		accountRef = existingStub[0].Ref
		var stub struct {
			CreatedAt string `firestore:"createdAt"`
		}
		_ = existingStub[0].DataTo(&stub)
		if stub.CreatedAt != "" {
			existingCreatedAt = stub.CreatedAt
		}
	}

	memberRef := sharedDoc.Ref.Collection("members").Doc(userID)
	memberSnap, err := memberRef.Get(ctx)
	role := "MEMBER"
	joinedAt := nowISO()
	if err == nil && memberSnap.Exists() {
		var existingMember struct {
			Role     string `firestore:"role"`
			JoinedAt string `firestore:"joinedAt"`
		}
		_ = memberSnap.DataTo(&existingMember)
		if existingMember.Role != "" {
			role = existingMember.Role
		}
		if existingMember.JoinedAt != "" {
			joinedAt = existingMember.JoinedAt
		}
	} else if sharedData.OwnerUserID == userID {
		role = "OWNER"
	}

	now := nowISO()
	name := sharedData.Name
	if name == "" {
		name = "Keuangan Bersama"
	}
	ownerUserID := sharedData.OwnerUserID
	if ownerUserID == "" {
		ownerUserID = userID
	}

	batch := s.db.Batch()
	member := buildMember(userID, role, email, displayName, now)
	member.JoinedAt = joinedAt
	batch.Set(memberRef, member, firestore.MergeAll)
	batch.Set(accountRef, map[string]interface{}{
		"name":            name,
		"role":            role,
		"ownerUserId":     ownerUserID,
		"sharedAccountId": sharedDoc.Ref.ID,
		"createdAt":       existingCreatedAt,
		"updatedAt":       now,
	}, firestore.MergeAll)
	batch.Set(userRef, map[string]interface{}{
		"activeAccountId": accountRef.ID,
		"updatedAt":       now,
	}, firestore.MergeAll)

	if _, err := batch.Commit(ctx); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":         true,
		"accountId":       accountRef.ID,
		"sharedAccountId": sharedDoc.Ref.ID,
		"name":            name,
		"role":            role,
	}, nil
}

func normalizeInviteCode(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RemoveSharedAccountAccess lets a member leave or an owner delete an empty workspace.
func (s *Service) RemoveSharedAccountAccess(ctx context.Context, userID, accountID string) (map[string]interface{}, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrInvalidArgument
	}

	userRef := s.db.Collection("users").Doc(userID)
	userAccountRef := userRef.Collection("accounts").Doc(accountID)
	snap, err := userAccountRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	var accountData struct {
		Name            string `firestore:"name"`
		SharedAccountID string `firestore:"sharedAccountId"`
		Role            string `firestore:"role"`
	}
	if err := snap.DataTo(&accountData); err != nil {
		return nil, err
	}
	if accountData.SharedAccountID == "" {
		return nil, ErrNotSharedAccount
	}

	sharedRef := s.db.Collection("sharedAccounts").Doc(accountData.SharedAccountID)
	sharedSnap, err := sharedRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	var sharedData struct {
		Name        string `firestore:"name"`
		OwnerUserID string `firestore:"ownerUserId"`
	}
	_ = sharedSnap.DataTo(&sharedData)

	fallbackID, err := s.getFallbackUserAccountID(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if fallbackID == "" {
		return nil, ErrLastAccountRequired
	}

	accountName := strings.TrimSpace(accountData.Name)
	if accountName == "" {
		accountName = strings.TrimSpace(sharedData.Name)
	}
	if accountName == "" {
		accountName = "Keuangan Bersama"
	}
	now := nowISO()

	if sharedData.OwnerUserID == userID {
		members, err := sharedRef.Collection("members").Documents(ctx).GetAll()
		if err != nil {
			return nil, err
		}
		if len(members) > 1 {
			return nil, fmt.Errorf("%w: %d other members remain", ErrMembersStillPresent, len(members)-1)
		}
		if err := s.deleteSharedWorkspaceDocuments(ctx, userID, accountID, accountData.SharedAccountID); err != nil {
			return nil, err
		}
		if err := s.updateUserRoutingAfterSharedAccountChange(ctx, userID, accountID, fallbackID); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"success":           true,
			"action":            "DELETED",
			"accountId":         accountID,
			"sharedAccountId":   accountData.SharedAccountID,
			"name":              accountName,
			"fallbackAccountId": fallbackID,
			"updatedAt":         now,
		}, nil
	}

	batch := s.db.Batch()
	batch.Delete(sharedRef.Collection("members").Doc(userID))
	batch.Delete(userAccountRef)
	if _, err := batch.Commit(ctx); err != nil {
		return nil, err
	}
	if err := s.updateUserRoutingAfterSharedAccountChange(ctx, userID, accountID, fallbackID); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":           true,
		"action":            "LEFT",
		"accountId":         accountID,
		"sharedAccountId":   accountData.SharedAccountID,
		"name":              accountName,
		"fallbackAccountId": fallbackID,
		"updatedAt":         now,
	}, nil
}

func (s *Service) getFallbackUserAccountID(ctx context.Context, userID, excludedAccountID string) (string, error) {
	iter := s.db.Collection("users").Doc(userID).Collection("accounts").Documents(ctx)
	type acc struct {
		id        string
		createdAt string
	}
	var remaining []acc
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		if doc.Ref.ID == excludedAccountID {
			continue
		}
		var data struct {
			CreatedAt string `firestore:"createdAt"`
		}
		_ = doc.DataTo(&data)
		created := data.CreatedAt
		if created == "" {
			created = "9999-12-31T23:59:59.999Z"
		}
		remaining = append(remaining, acc{id: doc.Ref.ID, createdAt: created})
	}
	if len(remaining) == 0 {
		return "", nil
	}
	sort.Slice(remaining, func(i, j int) bool {
		if remaining[i].createdAt != remaining[j].createdAt {
			return remaining[i].createdAt < remaining[j].createdAt
		}
		return remaining[i].id < remaining[j].id
	})
	return remaining[0].id, nil
}

func (s *Service) updateUserRoutingAfterSharedAccountChange(ctx context.Context, userID, targetAccountID, fallbackAccountID string) error {
	userRef := s.db.Collection("users").Doc(userID)
	userSnap, err := userRef.Get(ctx)
	if err != nil && status.Code(err) != codes.NotFound {
		return err
	}
	now := nowISO()
	if userSnap != nil && userSnap.Exists() {
		var data struct {
			ActiveAccountID string `firestore:"activeAccountId"`
		}
		_ = userSnap.DataTo(&data)
		if data.ActiveAccountID == targetAccountID {
			if _, err := userRef.Set(ctx, map[string]interface{}{
				"activeAccountId": fallbackAccountID,
				"updatedAt":       now,
			}, firestore.MergeAll); err != nil {
				return err
			}
		}
	}

	tgRef := userRef.Collection("telegram_link").Doc("main")
	tgSnap, err := tgRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return err
	}
	if !tgSnap.Exists() {
		return nil
	}
	var tgData struct {
		DefaultAccountID string `firestore:"defaultAccountId"`
	}
	_ = tgSnap.DataTo(&tgData)
	if tgData.DefaultAccountID == targetAccountID {
		_, err = tgRef.Set(ctx, map[string]interface{}{
			"defaultAccountId": fallbackAccountID,
			"updatedAt":        now,
		}, firestore.MergeAll)
		return err
	}
	return nil
}

func (s *Service) deleteSharedWorkspaceDocuments(ctx context.Context, ownerUserID, accountID, sharedAccountID string) error {
	sharedRef := s.db.Collection("sharedAccounts").Doc(sharedAccountID)
	ownerAccountRef := s.db.Collection("users").Doc(ownerUserID).Collection("accounts").Doc(accountID)

	cols := []string{"members", "categories", "transactions", "plans", "budgets", "simulations", "debts"}
	for _, col := range cols {
		if err := deleteCollection(ctx, s.db, sharedRef.Collection(col)); err != nil {
			return err
		}
	}
	if _, err := ownerAccountRef.Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return err
	}
	if _, err := sharedRef.Delete(ctx); err != nil && status.Code(err) != codes.NotFound {
		return err
	}
	return nil
}

func deleteCollection(ctx context.Context, db *firestore.Client, col *firestore.CollectionRef) error {
	for {
		docs, err := col.Limit(400).Documents(ctx).GetAll()
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		batch := db.Batch()
		for _, d := range docs {
			batch.Delete(d.Ref)
		}
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
	}
}
