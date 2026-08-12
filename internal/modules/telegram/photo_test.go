package telegram

import (
	"strings"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
)

// receiptParseResult feeds the shared confirmation flow, so its output shape is
// what the auto-save gate and the draft preview depend on.
func TestReceiptParseResult_DescriptionPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		receipt domain.ReceiptData
		caption string
		want    string
	}{
		{
			name:    "notes_win",
			receipt: domain.ReceiptData{Notes: "Belanja grosir sembako", Merchant: "Toko Abang"},
			want:    "Belanja grosir sembako",
		},
		{
			name:    "merchant_fallback",
			receipt: domain.ReceiptData{Merchant: "Toko Abang"},
			want:    "Toko Abang",
		},
		{
			name:    "default_when_blank",
			receipt: domain.ReceiptData{},
			want:    "Struk belanja",
		},
		{
			name:    "caption_overrides",
			receipt: domain.ReceiptData{Notes: "Belanja grosir sembako", Merchant: "Toko Abang"},
			caption: "Oleh-oleh kantor",
			want:    "Oleh-oleh kantor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, description := receiptParseResult(tt.receipt, tt.caption)
			if description != tt.want {
				t.Errorf("description = %q, want %q", description, tt.want)
			}
			if len(parsed.Items) != 1 || parsed.Items[0].Description != tt.want {
				t.Errorf("draft description = %q, want %q", parsed.Items[0].Description, tt.want)
			}
		})
	}
}

// Receipts are model-extracted, so they must stay flagged as AI and carry the
// numeric score through for the gate to decide.
func TestReceiptParseResult_MarkedAIWithScore(t *testing.T) {
	receipt := domain.ReceiptData{
		Merchant: "Toko Abang", TotalAmount: 52000,
		CategorySuggestion: "Belanja Harian", ConfidenceScore: 95,
	}

	parsed, _ := receiptParseResult(receipt, "")

	if !parsed.UsedAI {
		t.Error("a receipt must stay marked as AI-extracted")
	}
	if parsed.ConfidenceScore != 95 {
		t.Errorf("ConfidenceScore = %d, want 95", parsed.ConfidenceScore)
	}

	item := parsed.Items[0]
	if item.Amount != 52000 {
		t.Errorf("Amount = %d, want 52000", item.Amount)
	}
	if item.CategoryHint != "Belanja Harian" {
		t.Errorf("CategoryHint = %q, want Belanja Harian", item.CategoryHint)
	}
	if item.SourceText != "Toko Abang" {
		t.Errorf("SourceText = %q, want the merchant", item.SourceText)
	}
}

// The model can still return verbose notes despite the prompt cap; the
// description must stay readable in the transaction list.
func TestShortenDescription(t *testing.T) {
	short := "Paket sei sapi berdua di SeIndonesia"
	if got := shortenDescription(short); got != short {
		t.Errorf("short input must pass through, got %q", got)
	}

	long := "Pembelian paket makanan Seipesial Berdua 50% di SeIndonesia (Sei Sapi Dan Ayam) yang berisi daging sapi, nasi, sambal, telur mata sapi, dan es teh."
	got := shortenDescription(long)
	if len(got) > maxDescriptionLen+3 {
		t.Errorf("shortened = %q (%d bytes), want within cap + ellipsis", got, len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("shortened = %q, want trailing ellipsis", got)
	}
	if strings.HasSuffix(got, " ") {
		t.Errorf("shortened = %q, must not end with a space", got)
	}
}

func TestReceiptParseResult_LongNotesAreShortened(t *testing.T) {
	receipt := domain.ReceiptData{
		Merchant: "SeIndonesia",
		Notes:    "Pembelian paket makanan Seipesial Berdua 50% di SeIndonesia (Sei Sapi Dan Ayam) yang berisi daging sapi, nasi, sambal, telur mata sapi, dan es teh.",
	}

	_, description := receiptParseResult(receipt, "")

	if len(description) > maxDescriptionLen+3 {
		t.Errorf("description = %q, want a shortened summary", description)
	}
}

// Copied on-screen confirmation text must degrade to a plain label.
func TestReceiptParseResult_ConfirmationNotesBecomeLabels(t *testing.T) {
	receipt := domain.ReceiptData{
		Merchant: "Treasury",
		Notes:    "Pembelian emas berhasil senilai Rp150.000",
	}

	_, description := receiptParseResult(receipt, "")

	if description != "Pembelian emas" {
		t.Errorf("description = %q, want %q", description, "Pembelian emas")
	}
}
