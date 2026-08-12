package telegram

import (
	"strings"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
)

func sessionItem(desc string) domain.TextTransactionSessionItem {
	return domain.TextTransactionSessionItem{
		Amount: 25000, Description: desc, CategoryID: "c1", CategoryName: "Makanan",
	}
}

func TestBuildDraftKeyboard_SingleItemHasNoRemoveButtons(t *testing.T) {
	kb := buildDraftKeyboard("abcd1234", []domain.TextTransactionSessionItem{sessionItem("Makan")})

	rows := kb["inline_keyboard"].([][]map[string]string)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0][0]["text"] != "✅ Simpan" {
		t.Errorf("save label = %q, want the singular form", rows[0][0]["text"])
	}
	if rows[0][0]["callback_data"] != "mtc_abcd1234" {
		t.Errorf("save data = %q", rows[0][0]["callback_data"])
	}
}

func TestBuildDraftKeyboard_MultiItemPairsRemoveButtons(t *testing.T) {
	items := []domain.TextTransactionSessionItem{
		sessionItem("Makan"), sessionItem("Parkir"), sessionItem("Kopi"),
	}

	rows := buildDraftKeyboard("abcd1234", items)["inline_keyboard"].([][]map[string]string)

	// One action row plus two remove rows (2 + 1).
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0][0]["text"] != "✅ Simpan Semua" {
		t.Errorf("save label = %q, want the plural form", rows[0][0]["text"])
	}
	if len(rows[1]) != 2 || len(rows[2]) != 1 {
		t.Errorf("remove rows = %d,%d, want 2,1", len(rows[1]), len(rows[2]))
	}
	if rows[2][0]["callback_data"] != "mtr_abcd1234_2" {
		t.Errorf("last remove data = %q, want mtr_abcd1234_2", rows[2][0]["callback_data"])
	}
}

// Telegram rejects callback_data over 64 bytes, which would make every button
// silently dead.
func TestBuildDraftKeyboard_CallbackDataFitsTelegramLimit(t *testing.T) {
	items := make([]domain.TextTransactionSessionItem, 30)
	for i := range items {
		items[i] = sessionItem("Item")
	}

	for _, row := range buildDraftKeyboard("abcd1234", items)["inline_keyboard"].([][]map[string]string) {
		for _, btn := range row {
			if len(btn["callback_data"]) > 64 {
				t.Errorf("callback_data %q is %d bytes, over Telegram's 64-byte limit",
					btn["callback_data"], len(btn["callback_data"]))
			}
		}
	}
}

func TestManualInputs_PassesCategoryIDAsOverride(t *testing.T) {
	got := manualInputs([]domain.TextTransactionSessionItem{sessionItem("Makan")}, nil)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].CategoryIDOverride != "c1" {
		t.Errorf("override = %q, want c1 — saving must not re-resolve the category",
			got[0].CategoryIDOverride)
	}
}

// Receipt drafts are single-item, so the attachment only ever needs the first
// slot — but pin it, because a multi-item write must never duplicate it.
func TestManualInputs_AttachmentRidesFirstItemOnly(t *testing.T) {
	attachment := &domain.Attachment{Path: "users/u1/accounts/a1/attachments/receipt_1.jpg"}
	items := []domain.TextTransactionSessionItem{sessionItem("Makan"), sessionItem("Parkir")}

	got := manualInputs(items, attachment)

	if got[0].Attachment != attachment {
		t.Error("the first item must carry the attachment")
	}
	if got[1].Attachment != nil {
		t.Error("later items must not carry the attachment")
	}
}

func TestDeterministicHint(t *testing.T) {
	categories := []domain.Category{
		{ID: "c1", Name: "Makanan", Type: domain.TransactionTypeExpense},
		{ID: "c2", Name: "Transport", Type: domain.TransactionTypeExpense},
	}

	tests := []struct {
		name string
		hint string
		want bool
	}{
		{name: "empty_hint", hint: "", want: false},
		{name: "exact_name", hint: "Makanan", want: true},
		{name: "alias_food", hint: "food", want: true},
		{name: "alias_transportation", hint: "transportation", want: true},
		{name: "unknown_word", hint: "zzzz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deterministicHint(tt.hint, categories); got != tt.want {
				t.Errorf("deterministicHint(%q) = %v, want %v", tt.hint, got, tt.want)
			}
		})
	}
}

// The auto-save gate stays closed for anything the LLM touched (ADR-011), with
// one exception (ADR-016): a receipt whose numeric confidence clears the
// threshold and whose category resolved without the classifier.
func TestAutoSaveGate(t *testing.T) {
	single := &domain.HybridTransactionParseResult{
		Items:  []domain.ParsedTransactionDraft{{Amount: 25000, Description: "makan"}},
		UsedAI: false,
	}

	if !transaction.ShouldAutoSave(single, false) {
		t.Error("single local-parsed item with a deterministic category should auto-save")
	}
	if transaction.ShouldAutoSave(single, true) {
		t.Error("a classifier-resolved category must not auto-save")
	}

	aiParsed := &domain.HybridTransactionParseResult{Items: single.Items, UsedAI: true}
	if transaction.ShouldAutoSave(aiParsed, false) {
		t.Error("an AI-parsed message without a confidence score must not auto-save")
	}

	receipt := &domain.HybridTransactionParseResult{
		Items: single.Items, UsedAI: true,
		ConfidenceScore: transaction.ReceiptAutoSaveConfidenceThreshold + 5,
	}
	if !transaction.ShouldAutoSave(receipt, false) {
		t.Error("a high-confidence receipt with a deterministic category should auto-save")
	}
	if transaction.ShouldAutoSave(receipt, true) {
		t.Error("a classifier-resolved category must block even a high-confidence receipt")
	}

	multi := &domain.HybridTransactionParseResult{
		Items: []domain.ParsedTransactionDraft{
			{Amount: 25000, Description: "makan"},
			{Amount: 5000, Description: "parkir"},
		},
	}
	if transaction.ShouldAutoSave(multi, false) {
		t.Error("a multi-item message must not auto-save")
	}
}

// Every session failure has to produce a message; an empty string would leave
// the button looking dead.
func TestSessionErrorMessage_AlwaysExplains(t *testing.T) {
	for _, err := range []error{
		ErrSessionNotFound, ErrSessionExpired, ErrSessionForeign,
		ErrSessionEmpty, ErrItemIndexOutOfRange,
	} {
		msg := sessionErrorMessage(err)
		if strings.TrimSpace(msg) == "" {
			t.Errorf("sessionErrorMessage(%v) is empty", err)
		}
	}
}

func TestFormatAccountStatus_MarksActiveAccount(t *testing.T) {
	accounts := []account.UserAccount{
		{ID: "a1", Name: "Pribadi"},
		{ID: "a2", Name: "Rumah Tangga"},
	}

	got := FormatAccountStatus("Rumah Tangga", accounts)

	if !strings.Contains(got, "✅ 2. Rumah Tangga") {
		t.Errorf("active account not marked:\n%s", got)
	}
	if !strings.Contains(got, "• 1. Pribadi") {
		t.Errorf("inactive account not listed plainly:\n%s", got)
	}
}

func TestFormatAccountStatus_NoActiveAccount(t *testing.T) {
	got := FormatAccountStatus("", []account.UserAccount{{ID: "a1", Name: "Pribadi"}})

	if !strings.Contains(got, "Belum dipilih") {
		t.Errorf("expected the no-selection label:\n%s", got)
	}
}

// The active account gets no button: switching to it would do nothing.
func TestBuildAccountKeyboard_SkipsActiveAccount(t *testing.T) {
	accounts := []account.UserAccount{
		{ID: "a1", Name: "Pribadi"},
		{ID: "a2", Name: "Rumah Tangga"},
	}

	rows := buildAccountKeyboard(accounts, "a1")["inline_keyboard"].([][]map[string]string)

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0][0]["callback_data"] != "switch_account:a2" {
		t.Errorf("callback = %q, want switch_account:a2", rows[0][0]["callback_data"])
	}
}

func TestBuildAccountKeyboard_SingleAccountHasNoButtons(t *testing.T) {
	rows := buildAccountKeyboard([]account.UserAccount{{ID: "a1", Name: "Pribadi"}}, "a1")["inline_keyboard"].([][]map[string]string)

	if len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

func TestParseIndexSuffix(t *testing.T) {
	id, index, ok := parseIndexSuffix("abcd1234_2")
	if !ok || id != "abcd1234" || index != 2 {
		t.Errorf("parseIndexSuffix = (%q, %d, %v), want (abcd1234, 2, true)", id, index, ok)
	}

	if _, _, ok := parseIndexSuffix("noindex"); ok {
		t.Error("a payload without an index must be rejected")
	}
	if _, _, ok := parseIndexSuffix("abcd_x"); ok {
		t.Error("a non-numeric index must be rejected")
	}
}
