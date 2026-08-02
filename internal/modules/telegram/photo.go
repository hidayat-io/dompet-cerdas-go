package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

// largestPhotoFileID picks the biggest rendition Telegram offers. The array is
// ordered smallest to largest, and the smaller ones are too low-resolution for
// the model to read a total off a thermal receipt.
func largestPhotoFileID(msg map[string]interface{}) string {
	sizes, ok := msg["photo"].([]interface{})
	if !ok || len(sizes) == 0 {
		return ""
	}

	last, ok := sizes[len(sizes)-1].(map[string]interface{})
	if !ok {
		return ""
	}
	fileID, _ := last["file_id"].(string)
	return fileID
}

// documentFileID accepts an image sent as a file rather than a photo, which is
// what "send without compression" produces.
func documentFileID(msg map[string]interface{}) string {
	doc, ok := msg["document"].(map[string]interface{})
	if !ok {
		return ""
	}

	mimeType, _ := doc["mime_type"].(string)
	if !strings.HasPrefix(mimeType, "image/") {
		return ""
	}

	fileID, _ := doc["file_id"].(string)
	return fileID
}

// handleReceiptPhoto turns a photographed receipt into a draft, reusing the
// confirmation flow that typed transactions go through.
//
// This differs from the legacy bot, which kept receipt drafts in their own
// receipt_sessions collection with a separate set of callback prefixes. Reusing
// the text session keeps one state machine, one set of buttons, and one atomic
// claim path; the user-visible steps are the same.
func (h *Handler) handleReceiptPhoto(ctx context.Context, telegramID int64, fileID, caption string) error {
	rc, err := h.replyContextForLink(ctx, telegramID)
	if err != nil {
		if errors.Is(err, errNotLinked) {
			return h.bot.SendMessage(ctx, telegramID,
				"🔗 Akun Telegram ini belum terhubung.\n\nKetik /start untuk menghubungkannya dengan akun DompetCerdas kamu.", "Markdown")
		}
		slog.Error("telegram photo: no account context", "telegramId", telegramID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}

	if h.analyzer == nil {
		return h.send(ctx, rc, notPortedMessage("Scan struk"))
	}

	if err := h.bot.SendMessage(ctx, telegramID, "⏳ Membaca struk...", "Markdown"); err != nil {
		slog.Warn("telegram photo: failed to send progress notice", "error", err)
	}

	path, err := h.bot.GetFilePath(ctx, fileID)
	if err != nil {
		slog.Error("telegram photo: get file path failed", "error", err)
		return h.send(ctx, rc, "❌ Gagal mengunduh foto. Coba kirim ulang.")
	}

	raw, err := h.bot.DownloadFile(ctx, path)
	if err != nil {
		slog.Error("telegram photo: download failed", "error", err)
		return h.send(ctx, rc, "❌ Gagal mengunduh foto. Coba kirim ulang.")
	}

	receipt, err := transaction.AnalyzeReceipt(ctx, h.analyzer, raw)
	if err != nil {
		if errors.Is(err, transaction.ErrReceiptTooLarge) {
			return h.send(ctx, rc, "❌ Foto terlalu besar (maksimal 5MB).")
		}
		slog.Error("telegram photo: analysis failed", "error", err)
		return h.send(ctx, rc, "❌ Gagal membaca struk. Coba foto ulang dengan pencahayaan lebih baik.")
	}

	if !receipt.IsReceipt {
		return h.send(ctx, rc, "🤔 Foto ini sepertinya bukan struk belanja. Coba kirim foto struk yang totalnya terlihat jelas.")
	}

	// The receipt's own notes are the most descriptive summary available; the
	// merchant name alone reads poorly in a transaction list.
	description := strings.TrimSpace(receipt.Notes)
	if description == "" {
		description = strings.TrimSpace(receipt.Merchant)
	}
	if description == "" {
		description = "Struk belanja"
	}
	if caption = strings.TrimSpace(caption); caption != "" {
		description = caption
	}

	parsed := &domain.HybridTransactionParseResult{
		Items: []domain.ParsedTransactionDraft{{
			Amount:       receipt.TotalAmount,
			Description:  description,
			CategoryHint: receipt.CategorySuggestion,
			SourceText:   receipt.Merchant,
		}},
		// A receipt is always model-extracted, so it never auto-saves: the user
		// confirms the amount the model read before anything is written.
		UsedAI: true,
	}

	return h.sendDraft(ctx, rc, parsed, description, domain.SessionSourceText)
}
