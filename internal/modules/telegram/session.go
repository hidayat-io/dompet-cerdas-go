package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/crypto"
)

// sessionCollection and sessionTTL mirror the legacy bot
// (bot/index.ts:34, 347).
const (
	sessionCollection = "text_transaction_sessions"
	sessionTTL        = 30 * time.Minute
	// sessionIDBytes yields the same 8-hex-character id the legacy bot used, so
	// callback_data stays well inside Telegram's 64-byte limit.
	sessionIDBytes = 4
)

var (
	// ErrSessionNotFound means the id is unknown — usually a draft from a
	// previous deployment, or one already cleaned up.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired covers both the TTL and a session that is no longer
	// pending, matching isTextTransactionSessionExpired (bot/index.ts:234).
	ErrSessionExpired = errors.New("session expired or already resolved")
	// ErrSessionForeign means a different Telegram account pressed the button.
	ErrSessionForeign = errors.New("session belongs to another telegram account")
	// ErrItemIndexOutOfRange is a remove-item press for an item that is gone.
	ErrItemIndexOutOfRange = errors.New("item index out of range")
	// ErrSessionEmpty means every item was removed, so there is nothing to save.
	ErrSessionEmpty = errors.New("session has no items left")
)

// SessionStore persists Telegram transaction drafts awaiting confirmation.
type SessionStore struct {
	db *firestore.Client
}

func NewSessionStore(db *firestore.Client) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) doc(sessionID string) *firestore.DocumentRef {
	return s.db.Collection(sessionCollection).Doc(sessionID)
}

// Create stores a new pending draft and returns its id.
func (s *SessionStore) Create(ctx context.Context, session domain.TextTransactionSession) (string, error) {
	if len(session.Items) == 0 {
		return "", ErrSessionEmpty
	}

	sessionID, err := crypto.SecureToken(sessionIDBytes)
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	session.Status = domain.SessionStatusPending
	session.CreatedAt = time.Now()
	if session.SourceType == "" {
		session.SourceType = domain.SessionSourceText
	}

	if _, err := s.doc(sessionID).Create(ctx, session); err != nil {
		return "", err
	}
	return sessionID, nil
}

// Get reads a session without changing it.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*domain.TextTransactionSession, error) {
	snap, err := s.doc(sessionID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	var session domain.TextTransactionSession
	if err := snap.DataTo(&session); err != nil {
		return nil, err
	}
	return &session, nil
}

// usable reports whether a session may still be acted on, mirroring
// isTextTransactionSessionExpired.
func usable(session *domain.TextTransactionSession) error {
	if session.Status != domain.SessionStatusPending {
		return ErrSessionExpired
	}
	if !session.CreatedAt.IsZero() && time.Since(session.CreatedAt) > sessionTTL {
		return ErrSessionExpired
	}
	return nil
}

// ClaimForSave atomically flips a pending session to confirmed and returns what
// it held, so exactly one press can proceed to writing transactions.
//
// The legacy bot read the session, wrote the transactions, and only then marked
// it — a window in which two quick presses both read "pending" and both wrote,
// duplicating every row. Claiming first closes that window. The trade-off is
// deliberate: if the write then fails, the draft is spent and the user has to
// retype, which is recoverable. A duplicated financial record is not.
func (s *SessionStore) ClaimForSave(ctx context.Context, sessionID string, telegramID int64) (*domain.TextTransactionSession, error) {
	var claimed domain.TextTransactionSession

	err := s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ref := s.doc(sessionID)

		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return ErrSessionNotFound
			}
			return err
		}

		var session domain.TextTransactionSession
		if err := snap.DataTo(&session); err != nil {
			return err
		}
		if session.TelegramID != telegramID {
			return ErrSessionForeign
		}
		if err := usable(&session); err != nil {
			return err
		}
		if len(session.Items) == 0 {
			return ErrSessionEmpty
		}

		claimed = session
		return tx.Set(ref, map[string]interface{}{
			"status":      string(domain.SessionStatusConfirmed),
			"confirmedAt": time.Now(),
			"updatedAt":   time.Now(),
		}, firestore.MergeAll)
	})
	if err != nil {
		return nil, err
	}

	return &claimed, nil
}

// Cancel marks a pending session cancelled. It is idempotent in effect: a second
// press finds the session no longer pending and gets ErrSessionExpired, which
// the caller reports as "already handled" rather than as a failure.
func (s *SessionStore) Cancel(ctx context.Context, sessionID string, telegramID int64) (*domain.TextTransactionSession, error) {
	var cancelled domain.TextTransactionSession

	err := s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ref := s.doc(sessionID)

		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return ErrSessionNotFound
			}
			return err
		}

		var session domain.TextTransactionSession
		if err := snap.DataTo(&session); err != nil {
			return err
		}
		if session.TelegramID != telegramID {
			return ErrSessionForeign
		}
		if err := usable(&session); err != nil {
			return err
		}

		cancelled = session
		return tx.Set(ref, map[string]interface{}{
			"status":      string(domain.SessionStatusCancelled),
			"cancelledAt": time.Now(),
			"updatedAt":   time.Now(),
		}, firestore.MergeAll)
	})
	if err != nil {
		return nil, err
	}

	return &cancelled, nil
}

// RemoveItem drops one draft line and returns the session as it now stands, so
// the caller can re-render the preview.
//
// Removing the last remaining item marks the session invalid rather than leaving
// an empty draft that a later Save press could act on.
func (s *SessionStore) RemoveItem(ctx context.Context, sessionID string, telegramID int64, index int) (*domain.TextTransactionSession, error) {
	var updated domain.TextTransactionSession

	err := s.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ref := s.doc(sessionID)

		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return ErrSessionNotFound
			}
			return err
		}

		var session domain.TextTransactionSession
		if err := snap.DataTo(&session); err != nil {
			return err
		}
		if session.TelegramID != telegramID {
			return ErrSessionForeign
		}
		if err := usable(&session); err != nil {
			return err
		}
		if index < 0 || index >= len(session.Items) {
			return ErrItemIndexOutOfRange
		}

		session.Items = append(session.Items[:index], session.Items[index+1:]...)

		payload := map[string]interface{}{
			"items":     session.Items,
			"updatedAt": time.Now(),
		}
		if len(session.Items) == 0 {
			session.Status = domain.SessionStatusInvalid
			payload["status"] = string(domain.SessionStatusInvalid)
			payload["invalidatedAt"] = time.Now()
		}

		updated = session
		return tx.Set(ref, payload, firestore.MergeAll)
	})
	if err != nil {
		return nil, err
	}

	return &updated, nil
}
