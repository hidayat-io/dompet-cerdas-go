package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
)

// Callback prefixes for the account and unlink flows, matching the legacy bot.
const (
	callbackSwitchAccount = "switch_account:"
	callbackConfirmUnlink = "confirm_unlink"
	callbackCancelUnlink  = "cancel_unlink"
)

// FormatAccountStatus lists the user's accounts and marks the active one,
// porting formatTelegramAccountStatus (responseFormatter.ts:619).
//
// The active account is matched by name, as in the legacy formatter. Two
// accounts sharing a name both show as active — an inherited cosmetic quirk that
// does not affect which account is actually used.
func FormatAccountStatus(activeName string, accounts []account.UserAccount) string {
	lines := make([]string, 0, len(accounts))
	for i, acc := range accounts {
		marker := "•"
		if acc.Name == activeName {
			marker = "✅"
		}
		lines = append(lines, fmt.Sprintf("%s %d. %s", marker, i+1, EscapeMarkdown(acc.Name)))
	}

	displayName := activeName
	if displayName == "" {
		displayName = "Belum dipilih"
	}

	return fmt.Sprintf("⚙️ *Akun Telegram*\n\nAkun aktif saat ini: *%s*\n\nDaftar akun:\n%s",
		EscapeMarkdown(displayName), strings.Join(lines, "\n"))
}

// FormatAccountUpdated confirms a switch.
func FormatAccountUpdated(accountName string) string {
	return "✅ *Akun Telegram berhasil diganti*\n\nSekarang bot akan memakai akun: *" + EscapeMarkdown(accountName) + "*"
}

// buildAccountKeyboard offers one button per account, one per row so long names
// stay readable. The active account is skipped: switching to it is a no-op.
func buildAccountKeyboard(accounts []account.UserAccount, activeID string) map[string]interface{} {
	var rows [][]map[string]string
	for _, acc := range accounts {
		if acc.ID == activeID {
			continue
		}
		rows = append(rows, []map[string]string{{
			"text":          "🔄 " + acc.Name,
			"callback_data": callbackSwitchAccount + acc.ID,
		}})
	}
	return map[string]interface{}{"inline_keyboard": rows}
}

// handleAccountCommand answers /akun with the account list and switch buttons.
func (h *Handler) handleAccountCommand(ctx context.Context, telegramID int64) error {
	rc, err := h.replyContextForLink(ctx, telegramID)
	if err != nil {
		if errors.Is(err, errNotLinked) {
			return h.bot.SendMessage(ctx, telegramID,
				"⚠️ Akun belum terhubung. Ketik /start untuk menghubungkan akun.", "Markdown")
		}
		slog.Error("telegram /akun: failed to resolve context", "telegramId", telegramID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}

	accounts, err := h.accountService.ListUserAccounts(ctx, rc.userID)
	if err != nil {
		slog.Error("telegram /akun: failed to list accounts", "userId", rc.userID, "error", err)
		return h.send(ctx, rc, "❌ Gagal mengambil daftar akun.")
	}

	body := WithAccountHeader(FormatAccountStatus(rc.accountName, accounts), rc.accountName)

	if len(accounts) <= 1 {
		return h.bot.SendMessage(ctx, telegramID, body, "Markdown")
	}

	_, err = h.bot.SendMessageWithKeyboard(ctx, telegramID, body, "Markdown",
		buildAccountKeyboard(accounts, rc.ac.AccountID))
	return err
}

// handleUnlinkCommand asks for confirmation before breaking the link, since the
// action cannot be undone without going through /start again.
func (h *Handler) handleUnlinkCommand(ctx context.Context, telegramID int64) error {
	userID, _, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil {
		slog.Error("telegram /unlink: link lookup failed", "telegramId", telegramID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}
	if userID == "" {
		return h.bot.SendMessage(ctx, telegramID,
			"⚠️ Akun kamu belum terhubung dengan DompetCerdas.\n\nKetik /start untuk menghubungkan akun.", "Markdown")
	}

	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{{
			{"text": "✅ Ya, Disconnect", "callback_data": callbackConfirmUnlink},
			{"text": "❌ Batal", "callback_data": callbackCancelUnlink},
		}},
	}

	_, err = h.bot.SendMessageWithKeyboard(ctx, telegramID,
		"❓ *Konfirmasi Disconnect*\n\n"+
			"Apakah kamu yakin ingin memutuskan koneksi antara Telegram dan akun DompetCerdas?\n\n"+
			"Setelah disconnect, kamu bisa hubungkan lagi dengan akun yang berbeda menggunakan /start.",
		"Markdown", keyboard)
	return err
}

// handleSwitchAccount changes which account the bot reads and writes.
func (h *Handler) handleSwitchAccount(ctx context.Context, telegramID int64, messageID int, accountID string) error {
	userID, _, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil || userID == "" {
		return h.replaceDraftMessage(ctx, telegramID, messageID,
			"⚠️ Akun belum terhubung. Ketik /start untuk menghubungkan akun.")
	}

	accounts, err := h.accountService.ListUserAccounts(ctx, userID)
	if err != nil {
		slog.Error("telegram switch account: failed to list accounts", "userId", userID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, "❌ Terjadi kesalahan. Silakan coba lagi.")
	}

	// The id comes from a button the user pressed, but a stale keyboard could
	// still name an account that has since been removed, so it is verified
	// against the current list rather than trusted.
	var chosen *account.UserAccount
	for i := range accounts {
		if accounts[i].ID == accountID {
			chosen = &accounts[i]
			break
		}
	}
	if chosen == nil {
		return h.replaceDraftMessage(ctx, telegramID, messageID, "⚠️ Akun itu sudah tidak tersedia.")
	}

	if _, err := h.db.Collection("users").Doc(userID).
		Collection("telegram_link").Doc("main").
		Set(ctx, map[string]interface{}{
			"defaultAccountId":   chosen.ID,
			"defaultAccountName": chosen.Name,
			"updatedAt":          time.Now(),
		}, firestore.MergeAll); err != nil {
		slog.Error("telegram switch account: update failed", "userId", userID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, "❌ Gagal mengganti akun. Silakan coba lagi.")
	}

	slog.Info("telegram account switched", "userId", userID, "accountId", chosen.ID)
	return h.replaceDraftMessage(ctx, telegramID, messageID, FormatAccountUpdated(chosen.Name))
}

// handleConfirmUnlink deactivates the link. The document is kept with
// active=false rather than deleted, so re-linking and any audit trail survive.
func (h *Handler) handleConfirmUnlink(ctx context.Context, telegramID int64, messageID int) error {
	userID, _, err := h.linkService.GetTelegramLinkContext(ctx, telegramID)
	if err != nil {
		slog.Error("telegram unlink: link lookup failed", "telegramId", telegramID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, "❌ Terjadi kesalahan. Silakan coba lagi.")
	}
	if userID == "" {
		return h.replaceDraftMessage(ctx, telegramID, messageID,
			"⚠️ Akun kamu sudah tidak terhubung.")
	}

	if _, err := h.db.Collection("users").Doc(userID).
		Collection("telegram_link").Doc("main").
		Set(ctx, map[string]interface{}{
			"active":     false,
			"unlinkedAt": time.Now(),
			"updatedAt":  time.Now(),
		}, firestore.MergeAll); err != nil {
		slog.Error("telegram unlink: update failed", "userId", userID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, "❌ Gagal memutuskan koneksi. Silakan coba lagi.")
	}

	slog.Info("telegram account unlinked", "userId", userID, "telegramId", telegramID)
	return h.replaceDraftMessage(ctx, telegramID, messageID,
		"✅ *Koneksi diputus.*\n\nTelegram ini sudah tidak terhubung ke akun DompetCerdas.\n\nKetik /start kalau mau menghubungkan lagi.")
}

func (h *Handler) handleCancelUnlink(ctx context.Context, telegramID int64, messageID int) error {
	return h.replaceDraftMessage(ctx, telegramID, messageID, "👍 Dibatalkan. Koneksi tetap aktif.")
}

// parseIndexSuffix splits a "<id>_<index>" callback payload.
func parseIndexSuffix(payload string) (string, int, bool) {
	id, indexPart, ok := strings.Cut(payload, "_")
	if !ok {
		return "", 0, false
	}
	index, err := strconv.Atoi(indexPart)
	if err != nil {
		return "", 0, false
	}
	return id, index, true
}
