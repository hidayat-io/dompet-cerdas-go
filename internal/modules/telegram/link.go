package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/telegram/botapi"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/crypto"
)

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenUsed     = errors.New("token already used")
	ErrTokenExpired  = errors.New("token expired")
	ErrInvalidToken  = errors.New("invalid token")
)

// LinkService manages Telegram account linking.
type LinkService struct {
	db  *firestore.Client
	bot *botapi.Client
}

// NewLinkService constructs a link service.
func NewLinkService(db *firestore.Client, bot *botapi.Client) *LinkService {
	return &LinkService{db: db, bot: bot}
}

// GenerateLinkToken creates a one-time 5-minute link token for a Telegram user.
func (s *LinkService) GenerateLinkToken(ctx context.Context, telegramID int64, username, firstName, lastName string) (string, error) {
	token, err := crypto.SecureToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	doc := map[string]interface{}{
		"token":             token,
		"telegramId":        telegramID,
		"telegramUsername":  username,
		"telegramFirstName": firstName,
		"telegramLastName":  lastName,
		"createdAt":         now,
		"expiresAt":         now.Add(5 * time.Minute),
		"used":              false,
	}
	if _, err := s.db.Collection("link_tokens").Doc(token).Set(ctx, doc); err != nil {
		return "", err
	}
	return token, nil
}

type linkTokenDoc struct {
	Token             string    `firestore:"token"`
	TelegramID        int64     `firestore:"telegramId"`
	TelegramUsername  string    `firestore:"telegramUsername"`
	TelegramFirstName string    `firestore:"telegramFirstName"`
	TelegramLastName  string    `firestore:"telegramLastName"`
	CreatedAt         time.Time `firestore:"createdAt"`
	ExpiresAt         time.Time `firestore:"expiresAt"`
	Used              bool      `firestore:"used"`
}

// LinkAccount validates the token and links Telegram to the Firebase user.
func (s *LinkService) LinkAccount(ctx context.Context, userID, token string) (map[string]interface{}, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	ref := s.db.Collection("link_tokens").Doc(token)
	snap, err := ref.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	var data linkTokenDoc
	if err := snap.DataTo(&data); err != nil {
		return nil, err
	}
	if data.Used {
		return nil, ErrTokenUsed
	}
	if time.Now().After(data.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	defaultAccountID, _ := s.getActiveAccountID(ctx, userID)
	now := time.Now()

	if _, err := ref.Update(ctx, []firestore.Update{
		{Path: "used", Value: true},
		{Path: "usedAt", Value: now},
	}); err != nil {
		return nil, err
	}

	firstName := data.TelegramFirstName
	if firstName == "" {
		firstName = "Telegram"
	}
	link := domain.TelegramLink{
		TelegramID:       data.TelegramID,
		Username:         data.TelegramUsername,
		FirstName:        firstName,
		LastName:         data.TelegramLastName,
		DefaultAccountID: defaultAccountID,
		LinkedAt:         now,
		Active:           true,
		LastInteraction:  now,
	}
	// Store timestamps as Firestore-compatible values via map for linkedAt/lastInteraction
	// to match production Timestamp fields where possible.
	linkMap := map[string]interface{}{
		"telegramId":       data.TelegramID,
		"username":         data.TelegramUsername,
		"firstName":        firstName,
		"lastName":         data.TelegramLastName,
		"defaultAccountId": defaultAccountID,
		"linkedAt":         now,
		"active":           true,
		"lastInteraction":  now,
	}
	_ = link
	if _, err := s.db.Collection("users").Doc(userID).Collection("telegram_link").Doc("main").Set(ctx, linkMap); err != nil {
		return nil, err
	}

	accountName := ""
	if defaultAccountID != "" {
		accSnap, err := s.db.Collection("users").Doc(userID).Collection("accounts").Doc(defaultAccountID).Get(ctx)
		if err == nil && accSnap.Exists() {
			var acc struct {
				Name string `firestore:"name"`
			}
			_ = accSnap.DataTo(&acc)
			accountName = acc.Name
		}
	}

	return map[string]interface{}{
		"success":     true,
		"telegramId":  data.TelegramID,
		"accountId":   defaultAccountID,
		"accountName": accountName,
	}, nil
}

func (s *LinkService) getActiveAccountID(ctx context.Context, userID string) (string, error) {
	snap, err := s.db.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return "", err
	}
	var data struct {
		ActiveAccountID string `firestore:"activeAccountId"`
	}
	if err := snap.DataTo(&data); err != nil {
		return "", err
	}
	return data.ActiveAccountID, nil
}

// NotifyLinkSuccess sends the post-link confirmation to Telegram.
func (s *LinkService) NotifyLinkSuccess(ctx context.Context, userID string, telegramID int64, accountName string) error {
	if telegramID == 0 {
		return ErrInvalidToken
	}
	if accountName == "" {
		if id, err := s.getActiveAccountID(ctx, userID); err == nil && id != "" {
			accSnap, err := s.db.Collection("users").Doc(userID).Collection("accounts").Doc(id).Get(ctx)
			if err == nil && accSnap.Exists() {
				var acc struct {
					Name string `firestore:"name"`
				}
				_ = accSnap.DataTo(&acc)
				accountName = acc.Name
			}
		}
	}

	msg := "✅ *Akun berhasil terhubung!*\n\n"
	if accountName != "" {
		msg += fmt.Sprintf("Akun aktif saat ini: *%s*\n\n", EscapeMarkdown(accountName))
	}
	msg += "Sekarang kamu bisa:\n" +
		"• Tanya tentang keuangan kamu\n" +
		"• Upload foto struk untuk catat transaksi\n" +
		"• Lihat ringkasan pengeluaran\n\n" +
		"Ketik /help untuk panduan lengkap! 😊"

	if s.bot == nil {
		return errors.New("telegram bot client not configured")
	}
	return s.bot.SendMessage(ctx, telegramID, msg, "Markdown")
}

// GetTelegramLinkContext finds the active user linked to a Telegram ID.
func (s *LinkService) GetTelegramLinkContext(ctx context.Context, telegramID int64) (userID string, link map[string]interface{}, err error) {
	iter := s.db.CollectionGroup("telegram_link").
		Where("telegramId", "==", telegramID).
		Where("active", "==", true).
		Limit(1).
		Documents(ctx)
	doc, err := iter.Next()
	if err == iterator.Done {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	// path: users/{userId}/telegram_link/main
	parts := splitPath(doc.Ref.Path)
	// After projects/.../documents/users/{uid}/telegram_link/main
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "users" && i+1 < len(parts) {
			userID = parts[i+1]
			break
		}
	}
	return userID, doc.Data(), nil
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (s *LinkService) UpdateLastInteraction(ctx context.Context, telegramID int64) error {
	userID, _, err := s.GetTelegramLinkContext(ctx, telegramID)
	if err != nil || userID == "" {
		return err
	}
	_, err = s.db.Collection("users").Doc(userID).Collection("telegram_link").Doc("main").Set(ctx, map[string]interface{}{
		"lastInteraction": time.Now(),
	}, firestore.MergeAll)
	return err
}
