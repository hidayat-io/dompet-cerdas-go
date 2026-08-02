package domain

// TransactionType distinguishes income from expense.
type TransactionType string

const (
	// TransactionTypeIncome marks inbound money.
	TransactionTypeIncome TransactionType = "INCOME"
	// TransactionTypeExpense marks outbound money.
	TransactionTypeExpense TransactionType = "EXPENSE"
)

// TransactionSource indicates how a transaction was created.
type TransactionSource string

const (
	// TransactionSourceApp means created via the web UI.
	TransactionSourceApp TransactionSource = "app"
	// TransactionSourceTelegram means created via the Telegram bot.
	TransactionSourceTelegram TransactionSource = "telegram"
)

// AttachmentType is the media type of a transaction attachment.
type AttachmentType string

const (
	// AttachmentTypeImage is a raster image (JPEG/PNG/WebP).
	AttachmentTypeImage AttachmentType = "image"
	// AttachmentTypePDF is a PDF document.
	AttachmentTypePDF AttachmentType = "pdf"
)

// Transaction is a single income or expense entry.
// Firestore path: resolved by AccountContext.CollectionPath("transactions")
type Transaction struct {
	ID          string `firestore:"id"          json:"id"`
	Amount      int64  `firestore:"amount"      json:"amount"`
	Date        string `firestore:"date"        json:"date"` // YYYY-MM-DD in Jakarta time
	Description string `firestore:"description" json:"description"`
	CategoryID  string `firestore:"categoryId"  json:"categoryId"`

	CreatedAt string `firestore:"createdAt,omitempty" json:"createdAt,omitempty"` // ISO timestamp

	// Ownership/audit fields for shared-account permission model.
	CreatedByUserID string `firestore:"createdByUserId,omitempty" json:"createdByUserId,omitempty"`
	CreatedByName   string `firestore:"createdByName,omitempty"   json:"createdByName,omitempty"`
	UpdatedByUserID string `firestore:"updatedByUserId,omitempty" json:"updatedByUserId,omitempty"`
	UpdatedByName   string `firestore:"updatedByName,omitempty"   json:"updatedByName,omitempty"`

	Source TransactionSource `firestore:"source,omitempty" json:"source,omitempty"`

	// Structured attachment (current format).
	Attachment *Attachment `firestore:"attachment,omitempty" json:"attachment,omitempty"`

	// Legacy attachment fields — kept for backward compatibility with old documents.
	AttachmentURL  string         `firestore:"attachmentUrl,omitempty"  json:"attachmentUrl,omitempty"`
	AttachmentName string         `firestore:"attachmentName,omitempty" json:"attachmentName,omitempty"`
	AttachmentType AttachmentType `firestore:"attachmentType,omitempty" json:"attachmentType,omitempty"`
}

// Attachment is the structured attachment format for transaction documents.
type Attachment struct {
	URL  string         `firestore:"url"  json:"url"`
	Path string         `firestore:"path" json:"path"` // Firebase Storage path
	Type AttachmentType `firestore:"type" json:"type"`
	Name string         `firestore:"name" json:"name"`
	Size int64          `firestore:"size" json:"size"` // bytes
}

// Category groups transactions by type (income/expense) with a display icon and color.
// Firestore path: resolved by AccountContext.CollectionPath("categories")
type Category struct {
	ID              string          `firestore:"id"                          json:"id"`
	Name            string          `firestore:"name"                        json:"name"`
	Type            TransactionType `firestore:"type"                        json:"type"`
	Icon            string          `firestore:"icon"                        json:"icon"`  // Material icon name
	Color           string          `firestore:"color"                       json:"color"` // Hex color
	CreatedByUserID string          `firestore:"createdByUserId,omitempty"   json:"createdByUserId,omitempty"`
	CreatedByName   string          `firestore:"createdByName,omitempty"     json:"createdByName,omitempty"`
	Order           int             `firestore:"order,omitempty"             json:"order,omitempty"`
}

// Budget is a monthly spending limit tied to one or more categories.
// Firestore path: resolved by AccountContext.CollectionPath("budgets")
type Budget struct {
	ID          string   `firestore:"id"          json:"id"`
	Month       string   `firestore:"month"       json:"month"` // YYYY-MM
	Name        string   `firestore:"name"        json:"name"`
	CategoryIDs []string `firestore:"categoryIds" json:"categoryIds"`
	LimitAmount int64    `firestore:"limitAmount"  json:"limitAmount"`
	CreatedAt   string   `firestore:"createdAt"   json:"createdAt"`
	UpdatedAt   string   `firestore:"updatedAt"   json:"updatedAt"`

	CreatedByUserID string `firestore:"createdByUserId,omitempty" json:"createdByUserId,omitempty"`
	CreatedByName   string `firestore:"createdByName,omitempty"   json:"createdByName,omitempty"`

	// Legacy single-category field, kept for backward compatibility.
	CategoryID string `firestore:"categoryId,omitempty" json:"categoryId,omitempty"`
}

// PlanItemStatus tracks whether a plan item is still planned, done, or cancelled.
type PlanItemStatus string

const (
	// PlanItemStatusPlanned means the item has not been executed yet.
	PlanItemStatusPlanned PlanItemStatus = "PLANNED"
	// PlanItemStatusDone means the item has been executed.
	PlanItemStatusDone PlanItemStatus = "DONE"
	// PlanItemStatusCancelled means the item was cancelled.
	PlanItemStatusCancelled PlanItemStatus = "CANCELLED"
)

// PlanItem is a single line item within a financial plan.
type PlanItem struct {
	ID              string          `firestore:"id"                          json:"id"`
	Name            string          `firestore:"name"                        json:"name"`
	Amount          int64           `firestore:"amount"                      json:"amount"`
	Type            TransactionType `firestore:"type"                        json:"type"`
	CategoryID      string          `firestore:"categoryId"                  json:"categoryId"`
	PlannedDate     string          `firestore:"plannedDate,omitempty"       json:"plannedDate,omitempty"` // YYYY-MM-DD
	Status          PlanItemStatus  `firestore:"status"                      json:"status"`
	CreatedByUserID string          `firestore:"createdByUserId,omitempty"   json:"createdByUserId,omitempty"`
	CreatedByName   string          `firestore:"createdByName,omitempty"     json:"createdByName,omitempty"`
}

// Plan is a financial plan with projected income/expense items.
// Firestore path: resolved by AccountContext.CollectionPath("plans")
// Note: legacy documents may be stored under "simulations" collection.
type Plan struct {
	ID                     string     `firestore:"id"                                json:"id"`
	Title                  string     `firestore:"title"                             json:"title"`
	Items                  []PlanItem `firestore:"items"                             json:"items"`
	CreatedAt              string     `firestore:"createdAt"                         json:"createdAt"`
	UseCurrentMonthBalance bool       `firestore:"useCurrentMonthBalance,omitempty"  json:"useCurrentMonthBalance,omitempty"`
	CreatedByUserID        string     `firestore:"createdByUserId,omitempty"         json:"createdByUserId,omitempty"`
	CreatedByName          string     `firestore:"createdByName,omitempty"           json:"createdByName,omitempty"`
}

// DebtKind distinguishes money owed by the user vs money owed to the user.
type DebtKind string

const (
	// DebtKindDebt means the user owes money to someone.
	DebtKindDebt DebtKind = "DEBT"
	// DebtKindReceivable means someone owes money to the user.
	DebtKindReceivable DebtKind = "RECEIVABLE"
)

// DebtStatus tracks the payment state of a debt record.
type DebtStatus string

const (
	// DebtStatusUnpaid means no payment has been made.
	DebtStatusUnpaid DebtStatus = "UNPAID"
	// DebtStatusPartial means some but not all has been paid.
	DebtStatusPartial DebtStatus = "PARTIAL"
	// DebtStatusPaid means the debt is fully settled.
	DebtStatusPaid DebtStatus = "PAID"
)

// DebtPayment records a single payment against a debt.
type DebtPayment struct {
	ID        string `firestore:"id"        json:"id"`
	Amount    int64  `firestore:"amount"    json:"amount"`
	Date      string `firestore:"date"      json:"date"` // YYYY-MM-DD
	Note      string `firestore:"note,omitempty" json:"note,omitempty"`
	CreatedAt string `firestore:"createdAt" json:"createdAt"`
}

// DebtRecord is a debt or receivable entry with payment tracking.
// Firestore path: resolved by AccountContext.CollectionPath("debts")
type DebtRecord struct {
	ID              string        `firestore:"id"              json:"id"`
	Kind            DebtKind      `firestore:"kind"            json:"kind"`
	PersonName      string        `firestore:"personName"      json:"personName"`
	Title           string        `firestore:"title"           json:"title"`
	Amount          int64         `firestore:"amount"          json:"amount"`
	PaidAmount      int64         `firestore:"paidAmount"      json:"paidAmount"`
	RemainingAmount int64         `firestore:"remainingAmount" json:"remainingAmount"`
	Status          DebtStatus    `firestore:"status"          json:"status"`
	TransactionDate string        `firestore:"transactionDate" json:"transactionDate"` // YYYY-MM-DD
	DueDate         string        `firestore:"dueDate,omitempty" json:"dueDate,omitempty"`
	Notes           string        `firestore:"notes,omitempty"   json:"notes,omitempty"`
	Payments        []DebtPayment `firestore:"payments"        json:"payments"`
	CreatedAt       string        `firestore:"createdAt"       json:"createdAt"`
	UpdatedAt       string        `firestore:"updatedAt"       json:"updatedAt"`
	CreatedByUserID string        `firestore:"createdByUserId,omitempty" json:"createdByUserId,omitempty"`
	CreatedByName   string        `firestore:"createdByName,omitempty"   json:"createdByName,omitempty"`
}

// ReminderType controls when routine expense reminders fire.
type ReminderType string

const (
	// ReminderTypeAwalBulan triggers at the start of the month.
	ReminderTypeAwalBulan ReminderType = "AWAL_BULAN"
	// ReminderTypeAkhirBulan triggers at the end of the month.
	ReminderTypeAkhirBulan ReminderType = "AKHIR_BULAN"
	// ReminderTypeCustom triggers on a specific day.
	ReminderTypeCustom ReminderType = "CUSTOM"
)

// RoutineExpense is a recurring expense template.
// Firestore path: resolved by AccountContext.CollectionPath("routine_expenses")
type RoutineExpense struct {
	ID              string       `firestore:"id"                          json:"id"`
	Name            string       `firestore:"name"                        json:"name"`
	Amount          int64        `firestore:"amount"                      json:"amount"`
	CategoryID      string       `firestore:"categoryId"                  json:"categoryId"`
	CreatedAt       string       `firestore:"createdAt"                   json:"createdAt"`
	CreatedByUserID string       `firestore:"createdByUserId,omitempty"   json:"createdByUserId,omitempty"`
	CreatedByName   string       `firestore:"createdByName,omitempty"     json:"createdByName,omitempty"`
	ReminderEnabled bool         `firestore:"reminderEnabled,omitempty"   json:"reminderEnabled,omitempty"`
	ReminderType    ReminderType `firestore:"reminderType,omitempty"      json:"reminderType,omitempty"`
	ReminderDate    int          `firestore:"reminderDate,omitempty"      json:"reminderDate,omitempty"` // 1-31
	ReminderTime    string       `firestore:"reminderTime,omitempty"      json:"reminderTime,omitempty"` // e.g. "08:00"
}

// RoutineExpenseRecord tracks whether a routine expense was paid in a given month.
// Firestore path: resolved by AccountContext.CollectionPath("routine_expense_records")
type RoutineExpenseRecord struct {
	ID              string `firestore:"id"                        json:"id"`
	ExpenseID       string `firestore:"expenseId"                 json:"expenseId"`
	Month           string `firestore:"month"                     json:"month"` // YYYY-MM
	TransactionID   string `firestore:"transactionId,omitempty"   json:"transactionId,omitempty"`
	PaidAt          string `firestore:"paidAt"                    json:"paidAt"`
	CreatedByUserID string `firestore:"createdByUserId,omitempty" json:"createdByUserId,omitempty"`
}
