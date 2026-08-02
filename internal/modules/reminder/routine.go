package reminder

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/money"
)

// Reminder types stored on a routine expense document.
const (
	ReminderTypeStartOfMonth = "AWAL_BULAN"
	ReminderTypeEndOfMonth   = "AKHIR_BULAN"
	ReminderTypeCustom       = "CUSTOM"
)

// DefaultReminderTime is applied to routine expenses saved before the time field
// existed, matching the legacy `expense.reminderTime || '08:00'`.
const DefaultReminderTime = "08:00"

// routineExpense is the subset of a routine expense document the job needs.
type routineExpense struct {
	Name            string `firestore:"name"`
	Amount          int64  `firestore:"amount"`
	ReminderEnabled bool   `firestore:"reminderEnabled"`
	ReminderType    string `firestore:"reminderType"`
	ReminderDate    int    `firestore:"reminderDate"`
	ReminderTime    string `firestore:"reminderTime"`
	CreatedByUserID string `firestore:"createdByUserId"`
}

// HourLabel renders an hour as the "HH:00" string the reminder fields store.
func HourLabel(t time.Time) string {
	return t.In(datetime.Location()).Format("15") + ":00"
}

// MonthKey renders the "YYYY-MM" suffix used by routine expense payment records.
func MonthKey(t time.Time) string {
	return t.In(datetime.Location()).Format("2006-01")
}

// ShouldRemindToday decides whether a routine expense is due on the given day,
// porting the branch chain in processRoutineExpenseReminders
// (reminderService.ts:54-73).
//
// The CUSTOM overflow rule is deliberate: an expense due on the 31st would
// otherwise never fire in a 30-day month, so on the last day of the month any
// custom date still ahead is treated as due.
func ShouldRemindToday(exp routineExpense, day int, isStartOfMonth, isEndOfMonth bool, currentHour string) bool {
	should := false

	switch exp.ReminderType {
	case ReminderTypeStartOfMonth:
		should = isStartOfMonth
	case ReminderTypeEndOfMonth:
		should = isEndOfMonth
	case ReminderTypeCustom:
		should = exp.ReminderDate == day
	}

	if exp.ReminderType == ReminderTypeCustom && exp.ReminderDate > day && isEndOfMonth {
		should = true
	}

	expenseTime := exp.ReminderTime
	if expenseTime == "" {
		expenseTime = DefaultReminderTime
	}
	if expenseTime != currentHour {
		should = false
	}

	return should
}

// FormatRoutineExpenseReminder builds the notification text.
//
// The legacy message uses Intl currency formatting ("Rp 350.000"), which matches
// money.FormatExact, so the wording and the number both stay as users know them.
func FormatRoutineExpenseReminder(name string, amount int64) string {
	return "🔔 *Pengingat Pengeluaran Rutin*\n\n" +
		"Kamu punya tagihan/pengeluaran rutin yang belum dibayar untuk bulan ini:\n\n" +
		"📋 *" + name + "*\n" +
		"💰 *" + money.FormatExact(amount) + "*\n\n" +
		"Segera catat pembayarannya di aplikasi Dompet Cerdas."
}

// processRoutineExpenseReminders notifies users of unpaid recurring expenses,
// porting processRoutineExpenseReminders (reminderService.ts:9).
//
// One expense that fails to send must not abort the run, so per-document errors
// are logged and skipped rather than returned.
func (cm *CronManager) processRoutineExpenseReminders(ctx context.Context) {
	now := datetime.Now()
	day := now.Day()
	currentHour := HourLabel(now)
	monthKey := MonthKey(now)

	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, datetime.Location())
	lastDayOfMonth := firstOfMonth.AddDate(0, 1, -1).Day()
	isStartOfMonth := day == 1
	isEndOfMonth := day == lastDayOfMonth

	iter := cm.db.CollectionGroup("routine_expenses").
		Where("reminderEnabled", "==", true).
		Documents(ctx)
	defer iter.Stop()

	sent, skipped := 0, 0

	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			slog.Error("routine reminders: query failed", "error", err)
			return
		}

		var exp routineExpense
		if err := snap.DataTo(&exp); err != nil {
			slog.Warn("routine reminders: malformed document", "path", snap.Ref.Path, "error", err)
			continue
		}

		if !ShouldRemindToday(exp, day, isStartOfMonth, isEndOfMonth, currentHour) {
			skipped++
			continue
		}

		// routine_expenses lives under an account document; its payment records
		// are a sibling collection on that same account.
		accountRef := parentDocument(snap.Ref)
		if accountRef == nil || exp.CreatedByUserID == "" {
			continue
		}

		recordID := snap.Ref.ID + "_" + monthKey
		if _, err := accountRef.Collection("routine_expense_records").Doc(recordID).Get(ctx); err == nil {
			// Already recorded as paid this month.
			skipped++
			continue
		} else if status.Code(err) != codes.NotFound {
			slog.Error("routine reminders: failed to check payment record", "recordId", recordID, "error", err)
			continue
		}

		telegramID, ok := cm.activeTelegramID(ctx, exp.CreatedByUserID)
		if !ok {
			continue
		}

		if err := cm.bot.SendMessage(ctx, telegramID, FormatRoutineExpenseReminder(exp.Name, exp.Amount), "Markdown"); err != nil {
			slog.Error("routine reminders: send failed", "telegramId", telegramID, "expense", exp.Name, "error", err)
			continue
		}

		sent++
		slog.Info("routine reminder sent", "expense", exp.Name, "userId", exp.CreatedByUserID, "telegramId", telegramID)
	}

	slog.Info("routine reminders finished", "sent", sent, "skipped", skipped, "hour", currentHour)
}

// activeTelegramID resolves a user's live Telegram link, reporting false when
// the user has none or has unlinked.
func (cm *CronManager) activeTelegramID(ctx context.Context, userID string) (int64, bool) {
	snap, err := cm.db.Collection("users").Doc(userID).
		Collection("telegram_link").Doc("main").Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			slog.Error("reminders: failed to read telegram link", "userId", userID, "error", err)
		}
		return 0, false
	}

	var link struct {
		TelegramID int64 `firestore:"telegramId"`
		Active     bool  `firestore:"active"`
	}
	if err := snap.DataTo(&link); err != nil {
		return 0, false
	}
	if !link.Active || link.TelegramID == 0 {
		return 0, false
	}
	return link.TelegramID, true
}

// parentDocument returns the document a subcollection hangs off, or nil at the
// root. Firestore's Go client exposes Parent only on collections, so the walk is
// explicit.
func parentDocument(ref *firestore.DocumentRef) *firestore.DocumentRef {
	if ref == nil || ref.Parent == nil {
		return nil
	}
	return ref.Parent.Parent
}

// userIDFromLinkPath extracts the owning user id from a telegram_link document
// path ("users/{userId}/telegram_link/main").
func userIDFromLinkPath(path string) string {
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "users" {
			return parts[i+1]
		}
	}
	return ""
}
