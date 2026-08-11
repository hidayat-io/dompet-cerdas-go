package transaction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

// TestShouldAttemptTransactionParsing_Fixtures replays the guard decisions
// captured from the legacy backend.
//
// A false positive here means a query is parsed as a transaction and may be
// saved, so both the accepted and rejected sets matter equally.
func TestShouldAttemptTransactionParsing_Fixtures(t *testing.T) {
	const fixture = "transaction_parsing_guard.json"

	var data struct {
		Cases []struct {
			Input  string `json:"input"`
			Output bool   `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		if got := ShouldAttemptTransactionParsing(c.Input); got != c.Output {
			t.Errorf("ShouldAttemptTransactionParsing(%q) = %v, want %v", c.Input, got, c.Output)
		}
	}
}

type stubTextParser struct {
	called bool
	result *domain.HybridTransactionParseResult
	err    error
}

func (s *stubTextParser) ParseTransaction(context.Context, string) (*domain.HybridTransactionParseResult, error) {
	s.called = true
	return s.result, s.err
}

func TestParseWithFallback_LocalParseWinsOverAI(t *testing.T) {
	stub := &stubTextParser{}
	got, err := ParseWithFallback(context.Background(), "makan 25rb", stub)
	if err != nil {
		t.Fatalf("ParseWithFallback: %v", err)
	}
	if got == nil || got.Items[0].Amount != 25_000 {
		t.Fatalf("result = %+v, want local transaction", got)
	}
	if stub.called {
		t.Error("AI parser was called despite a successful local parse")
	}
}

func TestParseWithFallback_UsesAIWhenLocalParseFails(t *testing.T) {
	stub := &stubTextParser{result: &domain.HybridTransactionParseResult{
		Items:      []domain.ParsedTransactionDraft{{Amount: 6_000, Description: "air minum"}},
		Confidence: domain.ConfidenceHigh,
	}}
	got, err := ParseWithFallback(context.Background(), "tolong catat minum enam ribu", stub)
	if err != nil {
		t.Fatalf("ParseWithFallback: %v", err)
	}
	if !stub.called {
		t.Error("AI parser was not called after local parse failed")
	}
	if got == nil || !got.UsedAI {
		t.Fatalf("result = %+v, want UsedAI=true", got)
	}
	if ShouldAutoSave(got, false) {
		t.Error("AI result must never be eligible for auto-save")
	}
}

func TestParseWithFallback_DoesNotSendQueriesToAI(t *testing.T) {
	stub := &stubTextParser{}
	got, err := ParseWithFallback(context.Background(), "berapa pengeluaran 5 ribu minggu ini?", stub)
	if err != nil {
		t.Fatalf("ParseWithFallback: %v", err)
	}
	if got != nil {
		t.Errorf("result = %+v, want nil for query", got)
	}
	if stub.called {
		t.Error("AI parser was called for a query")
	}
}

func TestParseWithFallback_RejectsInvalidAIResult(t *testing.T) {
	tests := []struct {
		name   string
		result *domain.HybridTransactionParseResult
		err    error
	}{
		{name: "empty", result: &domain.HybridTransactionParseResult{}},
		{name: "partial", result: &domain.HybridTransactionParseResult{Items: []domain.ParsedTransactionDraft{{Amount: 10_000, Description: "makan"}, {Amount: 0, Description: "hilang"}}}},
		{name: "parser_error", err: errors.New("AI unavailable")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseWithFallback(context.Background(), "catat sepuluh ribu makan", &stubTextParser{result: tt.result, err: tt.err})
			if err == nil {
				t.Fatal("ParseWithFallback returned nil error for invalid AI result")
			}
		})
	}
}

// TestParseLocally_Fixtures replays parseTransactionMessageHybrid output.
//
// Cases recorded as "AI_FALLBACK" are inputs where the legacy local parser
// declined and deferred to the LLM. The Go local parser must decline at the same
// point rather than forcing a result, since a forced parse would bypass the
// preview-and-confirm flow.
func TestParseLocally_Fixtures(t *testing.T) {
	const fixture = "transaction_parsing.json"

	var data struct {
		Cases []struct {
			Input  string          `json:"input"`
			Output json.RawMessage `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		got := ParseLocally(c.Input)

		if isNullOrAIFallback(t, c.Output) {
			if got != nil {
				t.Errorf("ParseLocally(%q) = %+v, want nil (legacy declined locally)", c.Input, got)
			}
			continue
		}

		var want struct {
			Items []struct {
				Amount       int64  `json:"amount"`
				Description  string `json:"description"`
				CategoryHint string `json:"category_hint"`
				SourceText   string `json:"sourceText"`
			} `json:"items"`
			UsedAI     bool   `json:"usedAI"`
			Confidence string `json:"confidence"`
		}
		if err := json.Unmarshal(c.Output, &want); err != nil {
			t.Fatalf("case %q: cannot decode expected output: %v", c.Input, err)
		}

		if got == nil {
			t.Errorf("ParseLocally(%q) = nil, want %d items", c.Input, len(want.Items))
			continue
		}
		if got.UsedAI != want.UsedAI {
			t.Errorf("ParseLocally(%q).UsedAI = %v, want %v", c.Input, got.UsedAI, want.UsedAI)
		}
		if string(got.Confidence) != want.Confidence {
			t.Errorf("ParseLocally(%q).Confidence = %q, want %q", c.Input, got.Confidence, want.Confidence)
		}
		if len(got.Items) != len(want.Items) {
			t.Errorf("ParseLocally(%q) returned %d items, want %d", c.Input, len(got.Items), len(want.Items))
			continue
		}

		for i, wantItem := range want.Items {
			gotItem := got.Items[i]
			if gotItem.Amount != wantItem.Amount {
				t.Errorf("ParseLocally(%q) item %d Amount = %d, want %d",
					c.Input, i, gotItem.Amount, wantItem.Amount)
			}
			if gotItem.Description != wantItem.Description {
				t.Errorf("ParseLocally(%q) item %d Description = %q, want %q",
					c.Input, i, gotItem.Description, wantItem.Description)
			}
			if gotItem.CategoryHint != wantItem.CategoryHint {
				t.Errorf("ParseLocally(%q) item %d CategoryHint = %q, want %q",
					c.Input, i, gotItem.CategoryHint, wantItem.CategoryHint)
			}
			if gotItem.SourceText != wantItem.SourceText {
				t.Errorf("ParseLocally(%q) item %d SourceText = %q, want %q",
					c.Input, i, gotItem.SourceText, wantItem.SourceText)
			}
		}
	}
}

func isNullOrAIFallback(t *testing.T, raw json.RawMessage) bool {
	t.Helper()
	if string(raw) == "null" {
		return true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == "AI_FALLBACK"
	}
	return false
}

// TestQueryKeywordSubstringWart pins an inherited defect.
//
// containsQueryKeywords matches with strings.Contains, and "list" is a keyword,
// so "listrik" contains it. Legitimate transactions are silently rejected: the
// user sees no saved transaction and no error. Changing this alters what the bot
// accepts, which is a product decision, so it stays pinned here and visible.
func TestQueryKeywordSubstringWart(t *testing.T) {
	rejected := []string{
		"bayar listrik 350rb",
		"listrik 350rb",
		"beli listrik pln 100rb",
		"daftar belanja 50rb",
		"riwayat 10rb",
		"total 5rb",
	}
	for _, input := range rejected {
		if ShouldAttemptTransactionParsing(input) {
			t.Errorf("ShouldAttemptTransactionParsing(%q) = true, want false (inherited substring wart)", input)
		}
	}

	if !ShouldAttemptTransactionParsing("bayar air 50rb") {
		t.Error(`ShouldAttemptTransactionParsing("bayar air 50rb") = false, want true (no keyword substring)`)
	}
}

// TestExplicitEntryVerbOverridesQueryKeywords documents the escape hatch: an
// explicit entry verb bypasses the query-keyword rejection.
func TestExplicitEntryVerbOverridesQueryKeywords(t *testing.T) {
	if !ShouldAttemptTransactionParsing("catat 350rb bayar listrik") {
		t.Error("an explicit entry verb should bypass the query-keyword guard")
	}
}

// TestShouldAutoSave pins the auto-save gate. This is the highest-risk branch in
// the codebase: a true here writes a financial record with no user confirmation,
// so every condition is asserted independently.
func TestShouldAutoSave(t *testing.T) {
	oneItem := []domain.ParsedTransactionDraft{
		{Amount: 25000, Description: "makan siang", CategoryHint: "Food"},
	}
	twoItems := []domain.ParsedTransactionDraft{
		{Amount: 25000, Description: "makan"},
		{Amount: 5000, Description: "parkir"},
	}

	tests := []struct {
		name                 string
		result               *domain.HybridTransactionParseResult
		categoryResolvedByAI bool
		want                 bool
	}{
		{
			name:                 "single_item_local_parse_local_category",
			result:               &domain.HybridTransactionParseResult{Items: oneItem, UsedAI: false},
			categoryResolvedByAI: false,
			want:                 true,
		},
		{
			name:                 "multi_item_blocked",
			result:               &domain.HybridTransactionParseResult{Items: twoItems, UsedAI: false},
			categoryResolvedByAI: false,
			want:                 false,
		},
		{
			name:                 "ai_parse_blocked",
			result:               &domain.HybridTransactionParseResult{Items: oneItem, UsedAI: true},
			categoryResolvedByAI: false,
			want:                 false,
		},
		{
			name:                 "ai_category_blocked",
			result:               &domain.HybridTransactionParseResult{Items: oneItem, UsedAI: false},
			categoryResolvedByAI: true,
			want:                 false,
		},
		{
			name:                 "ai_parse_and_ai_category_blocked",
			result:               &domain.HybridTransactionParseResult{Items: oneItem, UsedAI: true},
			categoryResolvedByAI: true,
			want:                 false,
		},
		{
			name:                 "no_items_blocked",
			result:               &domain.HybridTransactionParseResult{Items: nil, UsedAI: false},
			categoryResolvedByAI: false,
			want:                 false,
		},
		{
			name:                 "nil_result_blocked",
			result:               nil,
			categoryResolvedByAI: false,
			want:                 false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldAutoSave(tt.result, tt.categoryResolvedByAI); got != tt.want {
				t.Errorf("ShouldAutoSave() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeParsedTransactionDrafts(t *testing.T) {
	in := []domain.ParsedTransactionDraft{
		{Amount: 25000, Description: "makan siang"},
		{Amount: 0, Description: "amount hilang"},
		{Amount: -500, Description: "amount negatif"},
		{Amount: 10000, Description: ""},
		{Amount: 10000, Description: "   "},
		{Amount: 5000, Description: "parkir"},
	}

	got := NormalizeParsedTransactionDrafts(in)

	if len(got) != 2 {
		t.Fatalf("got %d drafts, want 2: %+v", len(got), got)
	}
	if got[0].Description != "makan siang" || got[1].Description != "parkir" {
		t.Errorf("unexpected surviving drafts: %+v", got)
	}
}

// TestParseLocally_MultiItemSeparators covers each split path: newline,
// semicolon, comma, and conjunction. Comma and conjunction splitting only apply
// when more than one amount is present.
func TestParseLocally_MultiItemSeparators(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantItems int
	}{
		{name: "newline", input: "makan 25rb\nparkir 5rb", wantItems: 2},
		{name: "semicolon", input: "makan 25rb; parkir 5rb", wantItems: 2},
		{name: "comma", input: "kopi 18rb, parkir 5rb", wantItems: 2},
		{name: "conjunction_dan", input: "makan 25rb dan parkir 5rb", wantItems: 2},
		{name: "three_items", input: "hutang 10k, parkir 10k, beli hadiah 100k", wantItems: 3},
		{name: "single_item", input: "makan siang 25rb", wantItems: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLocally(tt.input)
			if got == nil {
				t.Fatalf("ParseLocally(%q) = nil, want %d items", tt.input, tt.wantItems)
			}
			if len(got.Items) != tt.wantItems {
				t.Errorf("ParseLocally(%q) returned %d items, want %d", tt.input, len(got.Items), tt.wantItems)
			}
		})
	}
}

// A comma inside a single-amount message is not a separator, so the description
// keeps it.
func TestParseLocally_CommaNotSplitWithSingleAmount(t *testing.T) {
	got := ParseLocally("beli kopi, gula 25rb")
	if got == nil {
		t.Fatal("ParseLocally returned nil")
	}
	if len(got.Items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(got.Items), got.Items)
	}
}

func TestParseLocally_IgnoresProductVolumeAsAmount(t *testing.T) {
	tests := []struct {
		input      string
		wantAmount int64
		wantDesc   string
	}{
		{input: "6k Le Minerale 1.5L", wantAmount: 6_000, wantDesc: "Le Minerale 1.5L"},
		{input: "6k Air Minum Le Minerale 1.5L", wantAmount: 6_000, wantDesc: "Air Minum Le Minerale 1.5L"},
		{input: "6k Beli Air Minum 1.5L", wantAmount: 6_000, wantDesc: "Beli Air Minum 1.5L"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLocally(tt.input)
			if got == nil || len(got.Items) != 1 {
				t.Fatalf("ParseLocally(%q) = %+v, want one item", tt.input, got)
			}
			if got.Items[0].Amount != tt.wantAmount {
				t.Errorf("amount = %d, want %d", got.Items[0].Amount, tt.wantAmount)
			}
			if got.Items[0].Description != tt.wantDesc {
				t.Errorf("description = %q, want %q", got.Items[0].Description, tt.wantDesc)
			}
		})
	}
}
