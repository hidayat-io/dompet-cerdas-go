package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
)

// Callback data prefixes for draft buttons, matching the legacy bot so that a
// keyboard sent before the migration still routes correctly.
const (
	callbackDraftSave   = "mtc_"
	callbackDraftCancel = "mtx_"
	callbackDraftRemove = "mtr_"
)

// buildDraftKeyboard renders the inline keyboard for a draft, mirroring
// buildTextTransactionDraftKeyboard (bot/index.ts).
//
// Remove buttons only appear for multi-item drafts, laid out two per row. The
// callback payloads stay short (prefix + 8-hex id + index) because Telegram caps
// callback_data at 64 bytes.
func buildDraftKeyboard(sessionID string, items []domain.TextTransactionSessionItem) map[string]interface{} {
	saveLabel := "✅ Simpan"
	if len(items) > 1 {
		saveLabel = "✅ Simpan Semua"
	}

	rows := [][]map[string]string{{
		{"text": saveLabel, "callback_data": callbackDraftSave + sessionID},
		{"text": "❌ Batal", "callback_data": callbackDraftCancel + sessionID},
	}}

	if len(items) > 1 {
		var row []map[string]string
		for i := range items {
			row = append(row, map[string]string{
				"text":          "🗑 Hapus " + strconv.Itoa(i+1),
				"callback_data": callbackDraftRemove + sessionID + "_" + strconv.Itoa(i),
			})
			if len(row) == 2 {
				rows = append(rows, row)
				row = nil
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}

	return map[string]interface{}{"inline_keyboard": rows}
}

// draftItems adapts session items for the formatters.
func draftItems(items []domain.TextTransactionSessionItem) []DraftItem {
	out := make([]DraftItem, 0, len(items))
	for _, item := range items {
		out = append(out, DraftItem{
			Amount:       item.Amount,
			Description:  item.Description,
			CategoryName: item.CategoryName,
		})
	}
	return out
}

// manualInputs adapts session items for the write path. The category id is
// passed as an override so that saving cannot re-resolve to a different category
// than the one the user just approved on screen.
func manualInputs(items []domain.TextTransactionSessionItem) []transaction.ManualInput {
	out := make([]transaction.ManualInput, 0, len(items))
	for _, item := range items {
		out = append(out, transaction.ManualInput{
			Amount:             item.Amount,
			Description:        item.Description,
			CategoryName:       item.CategoryName,
			CategoryIDOverride: item.CategoryID,
		})
	}
	return out
}

// resolveDraftItems attaches a category to every parsed draft and reports
// whether any of them needed the LLM, which is what disqualifies auto-save.
func (h *Handler) resolveDraftItems(
	ctx context.Context,
	rc replyContext,
	drafts []domain.ParsedTransactionDraft,
) (items []domain.TextTransactionSessionItem, usedClassifier bool, err error) {
	categories, err := h.accountRepo.GetUserCategories(ctx, rc.ac, false)
	if err != nil {
		return nil, false, err
	}

	for _, draft := range drafts {
		choice, err := transaction.ResolveCategoryChoice(ctx, h.classifier, draft.Description, categories, draft.CategoryHint)
		if err != nil {
			return nil, false, err
		}

		// A deterministic hit is exactly the "direct/alias match" ADR-011
		// requires for auto-save; anything else came from the classifier or the
		// fallback and must be confirmed by the user.
		if choice.Confidence != gemini.ConfidenceHigh || !deterministicHint(draft.CategoryHint, categories) {
			usedClassifier = true
		}

		items = append(items, domain.TextTransactionSessionItem{
			Amount:       draft.Amount,
			Description:  draft.Description,
			CategoryID:   choice.CategoryID,
			CategoryName: choice.CategoryName,
			SourceText:   draft.SourceText,
			CategoryHint: draft.CategoryHint,
		})
	}

	return items, usedClassifier, nil
}

// deterministicHint reports whether the hint alone could have selected a
// category without asking the model.
func deterministicHint(hint string, categories []domain.Category) bool {
	hint = strings.ToLower(strings.TrimSpace(hint))
	if hint == "" {
		return false
	}
	for _, cat := range categories {
		name := strings.ToLower(cat.Name)
		if name == hint || strings.Contains(name, hint) || strings.Contains(hint, name) {
			return true
		}
		for _, alias := range transaction.HintAliases(hint) {
			if strings.Contains(name, alias) {
				return true
			}
		}
	}
	return false
}

// sendDraft is the shared tail of every path that produces transactions from a
// message: it either saves silently when the bot is certain, or posts a preview
// with confirm buttons.
func (h *Handler) sendDraft(
	ctx context.Context,
	rc replyContext,
	parsed *domain.HybridTransactionParseResult,
	rawMessage string,
	sourceType domain.SessionSourceType,
) error {
	items, usedClassifier, err := h.resolveDraftItems(ctx, rc, parsed.Items)
	if err != nil {
		if errors.Is(err, transaction.ErrNoCategories) {
			return h.send(ctx, rc, "❌ Akun ini belum punya kategori. Buat dulu di aplikasi web DompetCerdas.")
		}
		slog.Error("telegram: failed to resolve draft categories", "userId", rc.userID, "error", err)
		return h.send(ctx, rc, "❌ Terjadi kesalahan. Silakan coba lagi.")
	}
	if len(items) == 0 {
		return h.send(ctx, rc, FormatUnknownIntent())
	}

	if transaction.ShouldAutoSave(parsed, usedClassifier) {
		return h.autoSave(ctx, rc, items[0])
	}

	sessionID, err := h.sessions.Create(ctx, domain.TextTransactionSession{
		UserID:      rc.userID,
		TelegramID:  rc.telegramID,
		AccountID:   rc.ac.AccountID,
		AccountName: rc.accountName,
		RawMessage:  rawMessage,
		SourceType:  sourceType,
		Items:       items,
		UsedAI:      parsed.UsedAI,
	})
	if err != nil {
		slog.Error("telegram: failed to create draft session", "userId", rc.userID, "error", err)
		return h.send(ctx, rc, "❌ Terjadi kesalahan. Silakan coba lagi.")
	}

	body := FormatTransactionDraftPreview(draftItems(items), parsed.UsedAI)
	if sourceType == domain.SessionSourceVoice {
		body = FormatVoiceTranscriptNote(rawMessage) + body
	}

	_, err = h.bot.SendMessageWithKeyboard(ctx, rc.telegramID,
		WithAccountHeader(body, rc.accountName), "Markdown", buildDraftKeyboard(sessionID, items))
	return err
}

// autoSave writes a single high-confidence transaction without asking.
//
// ADR-011 requires a structured audit line for every row that takes this path:
// the user never saw a confirmation, so the log is the only way to trace a
// mis-parse back to its input after the fact.
func (h *Handler) autoSave(ctx context.Context, rc replyContext, item domain.TextTransactionSessionItem) error {
	slog.Info("telegram auto-save",
		"autoSaveTriggered", true,
		"amount", item.Amount,
		"description", item.Description,
		"categoryName", item.CategoryName,
		"telegramId", rc.telegramID,
		"sourceText", item.SourceText,
	)

	if _, err := transaction.CreateManualBatch(ctx, h.db, h.accountService, h.accountRepo, rc.ac, rc.userID,
		manualInputs([]domain.TextTransactionSessionItem{item})); err != nil {
		slog.Error("telegram: auto-save write failed", "userId", rc.userID, "error", err)
		return h.send(ctx, rc, "❌ Gagal menyimpan transaksi. Silakan coba lagi.")
	}

	return h.send(ctx, rc, FormatAutoSavedTransaction(item.Amount, item.Description, item.CategoryName))
}

// sessionErrorMessage turns a session failure into something the user can act
// on. Every branch produces a reply, because a dead button with no explanation
// is indistinguishable from the bot being down.
func sessionErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return "⌛ Draft ini sudah tidak ada. Kirim ulang transaksinya ya."
	case errors.Is(err, ErrSessionExpired):
		return "⌛ Draft ini sudah kedaluwarsa atau sudah diproses. Kirim ulang transaksinya ya."
	case errors.Is(err, ErrSessionForeign):
		return "🚫 Draft ini milik akun Telegram lain."
	case errors.Is(err, ErrSessionEmpty):
		return "🗑 Semua item sudah dihapus, tidak ada yang bisa disimpan."
	case errors.Is(err, ErrItemIndexOutOfRange):
		return "🗑 Item itu sudah tidak ada di draft."
	default:
		return "❌ Terjadi kesalahan. Silakan coba lagi."
	}
}

// replaceDraftMessage rewrites the draft in place and drops its keyboard, so a
// resolved draft cannot be pressed again. If the edit fails — most often because
// the message is too old to edit — the outcome is still sent as a new message
// rather than lost.
func (h *Handler) replaceDraftMessage(ctx context.Context, telegramID int64, messageID int, body string) error {
	if messageID != 0 {
		if err := h.bot.EditMessageText(ctx, telegramID, messageID, body, "Markdown", nil); err == nil {
			return nil
		}
	}
	return h.bot.SendMessage(ctx, telegramID, body, "Markdown")
}

// handleDraftSave commits a draft. The session is claimed atomically first, so
// a double press cannot write the transactions twice.
func (h *Handler) handleDraftSave(ctx context.Context, telegramID int64, messageID int, sessionID string) error {
	session, err := h.sessions.ClaimForSave(ctx, sessionID, telegramID)
	if err != nil {
		slog.Info("telegram draft save rejected", "sessionId", sessionID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, sessionErrorMessage(err))
	}

	rc, err := h.replyContextFor(ctx, telegramID, session.AccountID, session.AccountName)
	if err != nil {
		slog.Error("telegram draft save: no account context", "sessionId", sessionID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, "❌ Terjadi kesalahan. Silakan coba lagi.")
	}

	if _, err := transaction.CreateManualBatch(ctx, h.db, h.accountService, h.accountRepo, rc.ac, session.UserID,
		manualInputs(session.Items)); err != nil {
		// The session is already spent. Say so plainly instead of implying a
		// retry will work on the same buttons.
		slog.Error("telegram draft save: write failed", "sessionId", sessionID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID,
			"❌ Gagal menyimpan transaksi. Draft ini hangus — silakan kirim ulang pesannya.")
	}

	body := WithAccountHeader(FormatTransactionBatchAdded(draftItems(session.Items)), session.AccountName)
	return h.replaceDraftMessage(ctx, telegramID, messageID, body)
}

func (h *Handler) handleDraftCancel(ctx context.Context, telegramID int64, messageID int, sessionID string) error {
	session, err := h.sessions.Cancel(ctx, sessionID, telegramID)
	if err != nil {
		slog.Info("telegram draft cancel rejected", "sessionId", sessionID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, sessionErrorMessage(err))
	}

	body := WithAccountHeader("❌ *Dibatalkan.* Tidak ada transaksi yang disimpan.", session.AccountName)
	return h.replaceDraftMessage(ctx, telegramID, messageID, body)
}

// handleDraftRemove drops one line and re-renders the draft with a keyboard that
// matches the new item count.
func (h *Handler) handleDraftRemove(ctx context.Context, telegramID int64, messageID int, payload string) error {
	sessionID, indexPart, ok := strings.Cut(payload, "_")
	if !ok {
		slog.Warn("telegram draft remove: malformed callback data", "payload", payload)
		return h.replaceDraftMessage(ctx, telegramID, messageID, sessionErrorMessage(ErrItemIndexOutOfRange))
	}

	index, err := strconv.Atoi(indexPart)
	if err != nil {
		slog.Warn("telegram draft remove: non-numeric index", "payload", payload)
		return h.replaceDraftMessage(ctx, telegramID, messageID, sessionErrorMessage(ErrItemIndexOutOfRange))
	}

	session, err := h.sessions.RemoveItem(ctx, sessionID, telegramID, index)
	if err != nil {
		slog.Info("telegram draft remove rejected", "sessionId", sessionID, "error", err)
		return h.replaceDraftMessage(ctx, telegramID, messageID, sessionErrorMessage(err))
	}

	if len(session.Items) == 0 {
		body := WithAccountHeader("🗑 Semua item dihapus. Tidak ada yang disimpan.", session.AccountName)
		return h.replaceDraftMessage(ctx, telegramID, messageID, body)
	}

	body := WithAccountHeader(FormatTransactionDraftPreview(draftItems(session.Items), session.UsedAI), session.AccountName)
	if messageID != 0 {
		if err := h.bot.EditMessageText(ctx, telegramID, messageID, body, "Markdown",
			buildDraftKeyboard(sessionID, session.Items)); err == nil {
			return nil
		}
	}

	_, err = h.bot.SendMessageWithKeyboard(ctx, telegramID, body, "Markdown", buildDraftKeyboard(sessionID, session.Items))
	return err
}
