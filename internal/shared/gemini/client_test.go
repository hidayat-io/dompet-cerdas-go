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
