package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

// Limits on an incoming voice note, ported from handleVoiceLikeMessage
// (bot/index.ts:440). Both are checked before the download so an oversized file
// is refused without spending bandwidth or a transcription call.
const (
	MaxVoiceDurationSeconds = 180
	MaxVoiceBytes           = 10 * 1024 * 1024
)

// Transcriber is the speech-to-text leg. Gemini is used here rather than the
// text model because it accepts the audio bytes directly.
type Transcriber interface {
	TranscribeAudio(ctx context.Context, audioBytes []byte, mimeType string) (string, error)
}

// voiceAttachment is the part of a message that carries audio.
type voiceAttachment struct {
	fileID   string
	mimeType string
	duration int
	fileSize int
}

// extractVoice pulls a voice note or an audio file from a message. Telegram
// sends these under different keys with different default mime types.
func extractVoice(msg map[string]interface{}) (voiceAttachment, bool) {
	for key, defaultMime := range map[string]string{"voice": "audio/ogg", "audio": "audio/mpeg"} {
		payload, ok := msg[key].(map[string]interface{})
		if !ok {
			continue
		}

		fileID, _ := payload["file_id"].(string)
		if fileID == "" {
			continue
		}

		mimeType, _ := payload["mime_type"].(string)
		if mimeType == "" {
			mimeType = defaultMime
		}

		return voiceAttachment{
			fileID:   fileID,
			mimeType: mimeType,
			duration: intFromAny(payload["duration"]),
			fileSize: intFromAny(payload["file_size"]),
		}, true
	}

	return voiceAttachment{}, false
}

func intFromAny(raw interface{}) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// handleVoiceMessage transcribes a voice note and routes the text through the
// same parser and draft flow a typed message takes.
//
// The draft always carries the transcript, so a mis-hearing is visible before
// the user confirms. That is also why a voice draft is never auto-saved —
// shouldAutoSaveDraft excludes the voice source: the transcription is a second
// place the meaning can drift, and an auto-saved reply would not show it.
func (h *Handler) handleVoiceMessage(ctx context.Context, telegramID int64, voice voiceAttachment) error {
	progressID := h.markProgress(ctx, telegramID, "🎤 Mendengarkan voice note...\n\nMohon tunggu beberapa detik...")
	defer h.clearProgress(ctx, telegramID, progressID)

	rc, err := h.replyContextForLink(ctx, telegramID)
	if err != nil {
		if errors.Is(err, errNotLinked) {
			return h.bot.SendMessage(ctx, telegramID,
				"🔗 Akun Telegram ini belum terhubung.\n\nKetik /start untuk menghubungkannya dengan akun DompetCerdas kamu.", "Markdown")
		}
		slog.Error("telegram voice: no account context", "telegramId", telegramID, "error", err)
		return h.bot.SendMessage(ctx, telegramID, "❌ Terjadi kesalahan. Silakan coba lagi.", "Markdown")
	}

	if h.transcriber == nil {
		return h.send(ctx, rc, notPortedMessage("Voice note"))
	}

	if voice.duration > MaxVoiceDurationSeconds {
		return h.send(ctx, rc, "⚠️ Voice note terlalu panjang.\n\nMaksimal 3 menit per kiriman ya.")
	}
	if voice.fileSize > MaxVoiceBytes {
		return h.send(ctx, rc, "⚠️ File audio terlalu besar.\n\nMaksimal 10MB per kiriman ya.")
	}

	path, err := h.bot.GetFilePath(ctx, voice.fileID)
	if err != nil {
		slog.Error("telegram voice: get file path failed", "error", err)
		return h.send(ctx, rc, "❌ Gagal mengunduh voice note. Coba kirim ulang.")
	}

	audio, err := h.bot.DownloadFile(ctx, path)
	if err != nil {
		slog.Error("telegram voice: download failed", "error", err)
		return h.send(ctx, rc, "❌ Gagal mengunduh voice note. Coba kirim ulang.")
	}

	// The size limit is re-checked against the real payload: Telegram's
	// file_size is advisory and absent on some clients.
	if len(audio) > MaxVoiceBytes {
		return h.send(ctx, rc, "⚠️ File audio terlalu besar.\n\nMaksimal 10MB per kiriman ya.")
	}

	transcript, err := h.transcriber.TranscribeAudio(ctx, audio, voice.mimeType)
	if err != nil {
		slog.Error("telegram voice: transcription failed", "error", err)
		return h.send(ctx, rc, "❌ Terjadi kesalahan saat memproses voice note. Silakan coba lagi.")
	}

	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return h.send(ctx, rc,
			"⚠️ Voice note belum berhasil dipahami.\n\nCoba ulang dengan ucapan yang lebih jelas, singkat, dan langsung sebut transaksi ya.")
	}

	parsed := transaction.ParseLocally(transcript)
	if parsed == nil || len(parsed.Items) == 0 {
		return h.send(ctx, rc, FormatVoiceNotUnderstood(transcript))
	}

	return h.sendDraft(ctx, rc, parsed, transcript, domain.SessionSourceVoice, receiptImage{})
}

// FormatVoiceNotUnderstood tells the user what was heard when the transcript
// parsed to nothing, so they can see whether the problem was the hearing or the
// phrasing.
func FormatVoiceNotUnderstood(transcript string) string {
	return "⚠️ Voice note berhasil ditranskrip, tapi belum terbaca sebagai transaksi.\n\n" +
		"🎤 Hasil suara: _" + EscapeMarkdown(transcript) + "_\n\n" +
		"Coba sebut lebih langsung, misalnya: _makan 25 ribu, parkir 5 ribu_."
}
