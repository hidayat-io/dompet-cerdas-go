package domain

import "time"

// SessionStatus tracks the lifecycle of a text or receipt transaction session.
type SessionStatus string

const (
	// SessionStatusPending means the user has not yet confirmed or cancelled.
	SessionStatusPending SessionStatus = "pending"
	// SessionStatusConfirmed means the user accepted the parsed transaction(s).
	SessionStatusConfirmed SessionStatus = "confirmed"
	// SessionStatusCancelled means the user rejected the parsed transaction(s).
	SessionStatusCancelled SessionStatus = "cancelled"
	// SessionStatusInvalid means the input could not be parsed into valid transactions.
	SessionStatusInvalid SessionStatus = "invalid"
)

// SessionSourceType indicates how the text message was received.
type SessionSourceType string

const (
	// SessionSourceText is a typed text message.
	SessionSourceText SessionSourceType = "text"
	// SessionSourceVoice is a transcribed voice note.
	SessionSourceVoice SessionSourceType = "voice"
)

// TextTransactionSessionItem is a single parsed draft within a text session.
type TextTransactionSessionItem struct {
	Amount       int64  `firestore:"amount"                    json:"amount"`
	Description  string `firestore:"description"               json:"description"`
	CategoryID   string `firestore:"categoryId"                json:"categoryId"`
	CategoryName string `firestore:"categoryName"              json:"categoryName"`
	SourceText   string `firestore:"sourceText"                json:"sourceText"`
	CategoryHint string `firestore:"category_hint,omitempty"   json:"category_hint,omitempty"`
}

// TextTransactionSession stores the state of a Telegram text-based transaction input.
// Firestore path: text_transaction_sessions/{sessionId}
type TextTransactionSession struct {
	UserID      string                       `firestore:"userId"                    json:"userId"`
	TelegramID  int64                        `firestore:"telegramId"                json:"telegramId"`
	AccountID   string                       `firestore:"accountId,omitempty"       json:"accountId,omitempty"`
	AccountName string                       `firestore:"accountName,omitempty"     json:"accountName,omitempty"`
	RawMessage  string                       `firestore:"rawMessage"                json:"rawMessage"`
	SourceType  SessionSourceType            `firestore:"sourceType,omitempty"      json:"sourceType,omitempty"`
	Items       []TextTransactionSessionItem `firestore:"items"                     json:"items"`
	UsedAI      bool                         `firestore:"usedAI"                    json:"usedAI"`
	// AttachmentFileID is the Telegram file_id of a receipt photo, set only for
	// receipt drafts. The photo is uploaded to Storage at confirmation time, so
	// a cancelled draft never leaves an orphaned object.
	AttachmentFileID string        `firestore:"attachmentFileId,omitempty" json:"attachmentFileId,omitempty"`
	Status           SessionStatus `firestore:"status"                    json:"status"`
	CreatedAt        time.Time     `firestore:"createdAt"                 json:"createdAt"`
	UpdatedAt        time.Time     `firestore:"updatedAt,omitempty"       json:"updatedAt,omitempty"`
	ConfirmedAt      time.Time     `firestore:"confirmedAt,omitempty"     json:"confirmedAt,omitempty"`
	CancelledAt      time.Time     `firestore:"cancelledAt,omitempty"     json:"cancelledAt,omitempty"`
	InvalidatedAt    time.Time     `firestore:"invalidatedAt,omitempty"   json:"invalidatedAt,omitempty"`
}

// ReceiptSession stores the state of a Telegram receipt-scan transaction input.
// Firestore path: receipt_sessions/{sessionId}
type ReceiptSession struct {
	UserID        string        `firestore:"userId"                    json:"userId"`
	TelegramID    int64         `firestore:"telegramId"                json:"telegramId"`
	ReceiptData   ReceiptData   `firestore:"receiptData"               json:"receiptData"`
	PhotoFileID   string        `firestore:"photoFileId"               json:"photoFileId"`
	PhotoCaption  string        `firestore:"photoCaption"              json:"photoCaption"`
	Status        SessionStatus `firestore:"status"                    json:"status"`
	Source        string        `firestore:"source,omitempty"          json:"source,omitempty"` // "document" for file uploads
	CreatedAt     time.Time     `firestore:"createdAt"                 json:"createdAt"`
	ExpiresAt     time.Time     `firestore:"expiresAt"                 json:"expiresAt"`
	ConfirmedAt   time.Time     `firestore:"confirmedAt,omitempty"     json:"confirmedAt,omitempty"`
	CancelledAt   time.Time     `firestore:"cancelledAt,omitempty"     json:"cancelledAt,omitempty"`
	TransactionID string        `firestore:"transactionId,omitempty"   json:"transactionId,omitempty"`
}
