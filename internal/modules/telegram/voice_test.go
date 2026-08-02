package telegram

import (
	"strings"
	"testing"
)

func TestExtractVoice_VoiceNote(t *testing.T) {
	msg := map[string]interface{}{
		"voice": map[string]interface{}{
			"file_id": "f1", "duration": float64(12), "file_size": float64(45000),
		},
	}

	got, ok := extractVoice(msg)
	if !ok {
		t.Fatal("a voice note should be extracted")
	}
	if got.fileID != "f1" || got.duration != 12 || got.fileSize != 45000 {
		t.Errorf("attachment = %+v", got)
	}
	if got.mimeType != "audio/ogg" {
		t.Errorf("mime = %q, want the voice default audio/ogg", got.mimeType)
	}
}

func TestExtractVoice_AudioFileUsesItsOwnDefault(t *testing.T) {
	msg := map[string]interface{}{
		"audio": map[string]interface{}{"file_id": "f2"},
	}

	got, ok := extractVoice(msg)
	if !ok {
		t.Fatal("an audio file should be extracted")
	}
	if got.mimeType != "audio/mpeg" {
		t.Errorf("mime = %q, want the audio default audio/mpeg", got.mimeType)
	}
}

func TestExtractVoice_ExplicitMimeWins(t *testing.T) {
	msg := map[string]interface{}{
		"voice": map[string]interface{}{"file_id": "f1", "mime_type": "audio/opus"},
	}

	got, _ := extractVoice(msg)
	if got.mimeType != "audio/opus" {
		t.Errorf("mime = %q, want audio/opus", got.mimeType)
	}
}

func TestExtractVoice_IgnoresOtherMessages(t *testing.T) {
	for _, msg := range []map[string]interface{}{
		{"text": "halo"},
		{"photo": []interface{}{}},
		{"voice": map[string]interface{}{}}, // present but no file_id
	} {
		if _, ok := extractVoice(msg); ok {
			t.Errorf("extractVoice(%v) should not match", msg)
		}
	}
}

// The transcript has to be shown back: without it the user cannot tell whether
// the bot misheard or they phrased it in a way the parser does not accept.
func TestFormatVoiceNotUnderstood_ShowsTheTranscript(t *testing.T) {
	got := FormatVoiceNotUnderstood("beli kopi tadi pagi")

	if !strings.Contains(got, "beli kopi tadi pagi") {
		t.Errorf("transcript missing from message:\n%s", got)
	}
}

func TestFormatVoiceNotUnderstood_EscapesTranscript(t *testing.T) {
	got := FormatVoiceNotUnderstood("beli buku *promo* [50%]")

	// The transcript is wrapped in italics; unescaped markdown inside it would
	// break Telegram's parser and the whole message would fail to send.
	if strings.Contains(got, "*promo*") {
		t.Errorf("markdown in the transcript was not escaped:\n%s", got)
	}
}

func TestVoiceLimits(t *testing.T) {
	if MaxVoiceDurationSeconds != 180 {
		t.Errorf("duration limit = %d, want 180 seconds", MaxVoiceDurationSeconds)
	}
	if MaxVoiceBytes != 10*1024*1024 {
		t.Errorf("size limit = %d, want 10MB", MaxVoiceBytes)
	}
}
