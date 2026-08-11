package gemini

import (
	"testing"
)

type DummyResult struct {
	Merchant string `json:"merchant"`
	Amount   int    `json:"amount"`
}

func TestParseJSONResult(t *testing.T) {
	raw := "Here is the result:\n```json\n{\n  \"merchant\": \"Toko Abang\",\n  \"amount\": 25000\n}\n```\nHope it helps!"

	res, err := ParseJSONResult[DummyResult](raw)
	if err != nil {
		t.Fatalf("ParseJSONResult failed: %v", err)
	}

	if res.Merchant != "Toko Abang" {
		t.Errorf("expected Toko Abang, got %q", res.Merchant)
	}
	if res.Amount != 25000 {
		t.Errorf("expected 25000, got %d", res.Amount)
	}
}

func TestNormalizeTransactionParseResponse(t *testing.T) {
	response := transactionParseResponse{
		Items: []transactionParseItem{
			{Amount: transactionAmount{String: "6k"}, Description: "  Air   Minum ", CategoryHint: "Food"},
			{Amount: transactionAmount{Int: int64Ptr(25_000)}, Description: "makan siang", SourceText: "makan 25rb"},
		},
		Confidence:          ConfidenceHigh,
		ClarificationNeeded: " ",
	}

	got, err := normalizeTransactionParseResponse(response, "pesan asli")
	if err != nil {
		t.Fatalf("normalizeTransactionParseResponse: %v", err)
	}
	if !got.UsedAI || got.Confidence != "high" {
		t.Fatalf("result = %+v, want AI/high", got)
	}
	if got.Items[0].Amount != 6_000 || got.Items[0].Description != "Air Minum" {
		t.Errorf("first item = %+v", got.Items[0])
	}
	if got.Items[0].SourceText != "pesan asli" {
		t.Errorf("default sourceText = %q, want original message", got.Items[0].SourceText)
	}
	if got.Items[1].Amount != 25_000 || got.Items[1].SourceText != "makan 25rb" {
		t.Errorf("second item = %+v", got.Items[1])
	}
}

func TestNormalizeTransactionParseResponse_RejectsInvalidItems(t *testing.T) {
	for _, item := range []transactionParseItem{
		{Amount: transactionAmount{String: "0"}, Description: "makan"},
		{Amount: transactionAmount{String: "6k"}, Description: "   "},
	} {
		if _, err := normalizeTransactionParseResponse(transactionParseResponse{Items: []transactionParseItem{item}}, "raw"); err == nil {
			t.Errorf("normalizeTransactionParseResponse(%+v) returned nil error", item)
		}
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestCleanDescription_StripsLeakedCurrency(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Transaksi pembelian emas berhasil senilai Rp150.000.", want: "Transaksi pembelian emas berhasil senilai"},
		{input: "pembelian emas Rp 150.000", want: "pembelian emas"},
		{input: "beli emas", want: "beli emas"},
		{input: "beli emas,,,", want: "beli emas"},
		{input: "pembelian emas Rp150.000. pada tanggal 2 Agustus 2026.", want: "pembelian emas"},
	}

	for _, tt := range tests {
		if got := cleanDescription(tt.input); got != tt.want {
			t.Errorf("cleanDescription(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeTransactionParseResponse_StripsCurrencyFromDescription(t *testing.T) {
	raw := `{"items":[{"amount":150000,"description":"pembelian emas Rp150.000.","category_hint":"Shopping","sourceText":"beli emas 150 ribu"}],"confidence":"high"}`

	resp, err := ParseJSONResult[transactionParseResponse](raw)
	if err != nil {
		t.Fatalf("ParseJSONResult: %v", err)
	}

	got, err := normalizeTransactionParseResponse(resp, "beli emas 150 ribu")
	if err != nil {
		t.Fatalf("normalizeTransactionParseResponse: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	if got.Items[0].Amount != 150_000 {
		t.Errorf("amount = %d, want 150000", got.Items[0].Amount)
	}
	if got.Items[0].Description != "pembelian emas" {
		t.Errorf("description = %q, want currency-stripped", got.Items[0].Description)
	}
	if got.Items[0].SourceText != "beli emas 150 ribu" {
		t.Errorf("sourceText = %q", got.Items[0].SourceText)
	}
	if !got.UsedAI {
		t.Error("UsedAI must be true for AI results")
	}
	if got.Confidence != "high" {
		t.Errorf("confidence = %q, want high", got.Confidence)
	}
}
