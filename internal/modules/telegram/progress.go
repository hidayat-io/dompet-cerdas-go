package telegram

import (
	"context"
	"log/slog"
)

// markProgress sends an immediate "received" bubble so the bot looks
// responsive while a download or LLM call runs. The returned id lets the
// caller remove the bubble once the real reply is ready; 0 means the bubble
// could not be sent and there is nothing to clean up.
func (h *Handler) markProgress(ctx context.Context, telegramID int64, text string) int {
	id, err := h.bot.SendMessageWithKeyboard(ctx, telegramID, text, "Markdown", nil)
	if err != nil {
		slog.Warn("telegram: failed to send progress notice", "telegramId", telegramID, "error", err)
		return 0
	}
	return id
}

// clearProgress removes the progress bubble after the real reply has been
// sent. A failure is cosmetic — the bubble then simply stays, like any old
// notice — so it never masks the real reply's outcome.
func (h *Handler) clearProgress(ctx context.Context, telegramID int64, messageID int) {
	if messageID == 0 {
		return
	}
	if err := h.bot.DeleteMessage(ctx, telegramID, messageID); err != nil {
		slog.Warn("telegram: failed to delete progress notice", "telegramId", telegramID, "error", err)
	}
}
