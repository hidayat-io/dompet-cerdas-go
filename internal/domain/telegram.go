package domain

import "time"

// TelegramLink represents the connection between a Firebase user and a Telegram account.
// Firestore path: users/{userId}/telegram_link (single document per user)
type TelegramLink struct {
	TelegramID       int64     `firestore:"telegramId"                    json:"telegramId"`
	Username         string    `firestore:"username,omitempty"            json:"username,omitempty"`
	FirstName        string    `firestore:"firstName"                     json:"firstName"`
	LastName         string    `firestore:"lastName,omitempty"            json:"lastName,omitempty"`
	DefaultAccountID string    `firestore:"defaultAccountId,omitempty"    json:"defaultAccountId,omitempty"`
	LinkedAt         time.Time `firestore:"linkedAt"                      json:"linkedAt"`
	Active           bool      `firestore:"active"                        json:"active"`
	LastInteraction  time.Time `firestore:"lastInteraction"               json:"lastInteraction"`
	ReminderEnabled  bool      `firestore:"reminderEnabled,omitempty"     json:"reminderEnabled,omitempty"`
	ReminderTime     string    `firestore:"reminderTime,omitempty"        json:"reminderTime,omitempty"` // e.g. "20:00"
	UnlinkedAt       time.Time `firestore:"unlinkedAt,omitempty"          json:"unlinkedAt,omitempty"`
	UpdatedAt        time.Time `firestore:"updatedAt,omitempty"           json:"updatedAt,omitempty"`
}

// LinkToken is a short-lived token used to connect a Telegram account to a Firebase user.
// Firestore path: link_tokens/{token}
type LinkToken struct {
	Token             string    `firestore:"token"                          json:"token"`
	TelegramID        int64     `firestore:"telegramId"                     json:"telegramId"`
	TelegramUsername  string    `firestore:"telegramUsername,omitempty"      json:"telegramUsername,omitempty"`
	TelegramFirstName string    `firestore:"telegramFirstName,omitempty"    json:"telegramFirstName,omitempty"`
	TelegramLastName  string    `firestore:"telegramLastName,omitempty"     json:"telegramLastName,omitempty"`
	CreatedAt         time.Time `firestore:"createdAt"                      json:"createdAt"`
	ExpiresAt         time.Time `firestore:"expiresAt"                      json:"expiresAt"`
	Used              bool      `firestore:"used"                           json:"used"`
	UsedAt            time.Time `firestore:"usedAt,omitempty"               json:"usedAt,omitempty"`
}
