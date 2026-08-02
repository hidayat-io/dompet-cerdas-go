package advisor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CooldownDuration  = 20 * time.Second
	DailyRequestLimit = 12
	DailyTokenLimit   = 30000
	EstimatedTokens   = 4700 // 3500 prompt + 1200 response
)

type QuotaManager struct {
	db *firestore.Client
}

type Reservation struct {
	DailyTokensUsed int
}

// ErrCooldown and ErrQuotaExceeded let callers tell a rate-limit refusal apart
// from a genuine failure, so the user is told to wait rather than shown a
// generic error.
var (
	ErrCooldown      = errors.New("analysis cooldown active")
	ErrQuotaExceeded = errors.New("daily analysis quota exceeded")
)

func NewQuotaManager(db *firestore.Client) *QuotaManager {
	return &QuotaManager{db: db}
}

func (qm *QuotaManager) Reserve(ctx context.Context, userID string) (*Reservation, error) {
	ref := qm.db.Collection("web_ai_limits").Doc(userID)
	var res Reservation
	now := time.Now()

	err := qm.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}

		var data struct {
			DayStart        time.Time `firestore:"dayStart"`
			DailyTokensUsed int       `firestore:"dailyTokensUsed"`
			DailyRequests   int       `firestore:"dailyRequests"`
			LastRequest     time.Time `firestore:"lastRequest"`
		}

		if snap != nil && snap.Exists() {
			snap.DataTo(&data)
		} else {
			data.DayStart = now
		}

		// Rolling 24h reset
		if now.Sub(data.DayStart) >= 24*time.Hour {
			data.DayStart = now
			data.DailyTokensUsed = 0
			data.DailyRequests = 0
		}

		if !data.LastRequest.IsZero() && now.Sub(data.LastRequest) < CooldownDuration {
			wait := int(CooldownDuration.Seconds() - now.Sub(data.LastRequest).Seconds())
			return fmt.Errorf("%w: mohon tunggu %d detik sebelum analisis berikutnya", ErrCooldown, wait)
		}

		if data.DailyRequests >= DailyRequestLimit {
			return fmt.Errorf("%w: batas analisis harian tercapai (%d analisis per hari)", ErrQuotaExceeded, DailyRequestLimit)
		}

		if data.DailyTokensUsed+EstimatedTokens > DailyTokenLimit {
			return fmt.Errorf("%w: batas token harian tercapai (%d token per hari)", ErrQuotaExceeded, DailyTokenLimit)
		}

		res.DailyTokensUsed = data.DailyTokensUsed

		docData := map[string]interface{}{
			"dayStart":        data.DayStart,
			"dailyTokensUsed": data.DailyTokensUsed,
			"dailyRequests":   data.DailyRequests + 1,
			"lastRequest":     now,
			"updatedAt":       now,
		}

		if snap == nil || !snap.Exists() {
			return tx.Set(ref, docData)
		}

		return tx.Update(ref, []firestore.Update{
			{Path: "dayStart", Value: data.DayStart},
			{Path: "dailyTokensUsed", Value: data.DailyTokensUsed},
			{Path: "dailyRequests", Value: data.DailyRequests + 1},
			{Path: "lastRequest", Value: now},
			{Path: "updatedAt", Value: now},
		})
	})

	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (qm *QuotaManager) Commit(ctx context.Context, userID string, previousUsed int, actualTokens int) error {
	ref := qm.db.Collection("web_ai_limits").Doc(userID)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "dailyTokensUsed", Value: previousUsed + actualTokens},
		{Path: "updatedAt", Value: time.Now()},
	})
	return err
}
