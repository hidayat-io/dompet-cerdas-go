package reminder

import (
	"context"
	"log/slog"

	"google.golang.org/api/iterator"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// DailyNoTransactionMessage is the nudge sent to users who recorded nothing
// today. Kept verbatim from the legacy bot so the reminder users already receive
// does not change wording mid-migration.
const DailyNoTransactionMessage = "⚠️ *Pengingat Dompet Cerdas*\n\n" +
	"Hari ini belum ada transaksi yang Anda input, apakah Anda melewatkan sesuatu?\n\n" +
	"Silakan ketik atau kirim foto struk transaksi Anda langsung di sini untuk mencatat! 📝"

// telegramLink is the subset of a link document the daily job filters on.
type telegramLink struct {
	TelegramID      int64  `firestore:"telegramId"`
	Active          bool   `firestore:"active"`
	ReminderEnabled bool   `firestore:"reminderEnabled"`
	ReminderTime    string `firestore:"reminderTime"`
}

// processDailyNoTransactionReminders nudges users who logged nothing today,
// porting processDailyNoTransactionReminders (reminderService.ts:118).
//
// The scan reads every telegram_link document and filters in memory. That is the
// legacy shape and it is O(all linked users) every hour; it is kept because
// adding a composite index on (active, reminderEnabled, reminderTime) changes
// deployment requirements, and the current user count makes it a non-issue.
// Revisit when the hourly run stops finishing quickly.
func (cm *CronManager) processDailyNoTransactionReminders(ctx context.Context) {
	now := datetime.Now()
	currentHour := HourLabel(now)
	today := datetime.TodayString()

	iter := cm.db.CollectionGroup("telegram_link").Documents(ctx)
	defer iter.Stop()

	sent, skipped := 0, 0

	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			slog.Error("daily reminders: query failed", "error", err)
			return
		}

		var link telegramLink
		if err := snap.DataTo(&link); err != nil {
			continue
		}
		if !link.Active || !link.ReminderEnabled || link.ReminderTime != currentHour || link.TelegramID == 0 {
			skipped++
			continue
		}

		userID := userIDFromLinkPath(snap.Ref.Path)
		if userID == "" {
			continue
		}

		recorded, err := cm.hasTransactionToday(ctx, userID, today)
		if err != nil {
			// Staying silent on a read error is the safer failure: a spurious
			// "you recorded nothing today" to someone who did is worse than a
			// missed nudge.
			slog.Error("daily reminders: transaction check failed", "userId", userID, "error", err)
			continue
		}
		if recorded {
			skipped++
			continue
		}

		if err := cm.bot.SendMessage(ctx, link.TelegramID, DailyNoTransactionMessage, "Markdown"); err != nil {
			slog.Error("daily reminders: send failed", "telegramId", link.TelegramID, "error", err)
			continue
		}

		sent++
		slog.Info("daily reminder sent", "userId", userID, "telegramId", link.TelegramID)
	}

	slog.Info("daily reminders finished", "sent", sent, "skipped", skipped, "hour", currentHour)
}

// hasTransactionToday reports whether the user recorded anything today in any of
// their accounts.
//
// Both the account-scoped paths and the legacy path are checked, because a user
// migrated mid-day can have today's rows under either shape. The filter is on
// createdByUserId, so in a shared account another member's entry does not
// suppress this user's reminder.
func (cm *CronManager) hasTransactionToday(ctx context.Context, userID, today string) (bool, error) {
	accounts, err := cm.accountService.ListUserAccounts(ctx, userID)
	if err != nil {
		return false, err
	}

	contexts := make([]*account.Context, 0, len(accounts)+1)
	for _, acc := range accounts {
		contexts = append(contexts, account.ContextForAccount(userID, acc))
	}
	contexts = append(contexts, account.LegacyContext(userID))

	for _, ac := range contexts {
		snaps, err := cm.accountService.GetTransactionsCollection(ac).
			Where("createdByUserId", "==", userID).
			Where("date", "==", today).
			Limit(1).
			Documents(ctx).GetAll()
		if err != nil {
			return false, err
		}
		if len(snaps) > 0 {
			return true, nil
		}
	}

	return false, nil
}
