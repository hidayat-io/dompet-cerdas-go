package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
)

func cat(id, name string, t domain.TransactionType) domain.Category {
	return domain.Category{ID: id, Name: name, Type: t}
}

var sampleCategories = []domain.Category{
	cat("c1", "Makanan", domain.TransactionTypeExpense),
	cat("c2", "Transport", domain.TransactionTypeExpense),
	cat("c3", "Belanja", domain.TransactionTypeExpense),
	cat("c4", "Lainnya", domain.TransactionTypeExpense),
	cat("c5", "Gaji", domain.TransactionTypeIncome),
}

// stubClassifier records whether it was called, so tests can prove the
// deterministic legs short-circuit before any network call.
type stubClassifier struct {
	called bool
	choice gemini.CategoryChoice
	err    error
}

func (s *stubClassifier) ClassifyCategory(context.Context, string, []gemini.CategoryCandidate) (gemini.CategoryChoice, error) {
	s.called = true
	return s.choice, s.err
}

func TestFallbackCategory_PrefersShoppingForExpense(t *testing.T) {
	got, err := FallbackCategory(sampleCategories, domain.TransactionTypeExpense)
	if err != nil {
		t.Fatalf("FallbackCategory: %v", err)
	}
	if got.ID != "c3" {
		t.Errorf("id = %s, want c3 (Belanja)", got.ID)
	}
}

func TestFallbackCategory_FallsToOtherWhenNoShopping(t *testing.T) {
	categories := []domain.Category{
		cat("c1", "Makanan", domain.TransactionTypeExpense),
		cat("c4", "Lainnya", domain.TransactionTypeExpense),
	}

	got, err := FallbackCategory(categories, domain.TransactionTypeExpense)
	if err != nil {
		t.Fatalf("FallbackCategory: %v", err)
	}
	if got.ID != "c4" {
		t.Errorf("id = %s, want c4 (Lainnya)", got.ID)
	}
}

// The shopping preference is expense-only; an income fallback must not land on
// a shopping category.
func TestFallbackCategory_IncomeSkipsShopping(t *testing.T) {
	categories := []domain.Category{
		cat("c3", "Belanja", domain.TransactionTypeExpense),
		cat("c5", "Gaji", domain.TransactionTypeIncome),
	}

	got, err := FallbackCategory(categories, domain.TransactionTypeIncome)
	if err != nil {
		t.Fatalf("FallbackCategory: %v", err)
	}
	if got.ID != "c5" {
		t.Errorf("id = %s, want c5 (Gaji)", got.ID)
	}
}

func TestFallbackCategory_LastResortIsFirstCategory(t *testing.T) {
	categories := []domain.Category{cat("c9", "Hiburan", domain.TransactionTypeExpense)}

	got, err := FallbackCategory(categories, domain.TransactionTypeIncome)
	if err != nil {
		t.Fatalf("FallbackCategory: %v", err)
	}
	if got.ID != "c9" {
		t.Errorf("id = %s, want c9", got.ID)
	}
}

func TestFallbackCategory_EmptyIsAnError(t *testing.T) {
	if _, err := FallbackCategory(nil, domain.TransactionTypeExpense); !errors.Is(err, ErrNoCategories) {
		t.Errorf("err = %v, want ErrNoCategories", err)
	}
}

func TestResolveCategoryChoice_ExactHintSkipsClassifier(t *testing.T) {
	stub := &stubClassifier{}

	got, err := ResolveCategoryChoice(context.Background(), stub, "makan siang", sampleCategories, "Makanan")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.CategoryID != "c1" || got.Confidence != gemini.ConfidenceHigh {
		t.Errorf("choice = %+v, want c1/high", got)
	}
	if stub.called {
		t.Error("classifier was called despite an exact hint match")
	}
}

func TestResolveCategoryChoice_AliasHintSkipsClassifier(t *testing.T) {
	stub := &stubClassifier{}

	// "food" is not a category name; it reaches Makanan through hintAliasMap.
	got, err := ResolveCategoryChoice(context.Background(), stub, "nasi padang", sampleCategories, "food")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.CategoryID != "c1" || got.Confidence != gemini.ConfidenceHigh {
		t.Errorf("choice = %+v, want c1/high", got)
	}
	if stub.called {
		t.Error("classifier was called despite an alias match")
	}
}

func TestResolveCategoryChoice_UsesClassifierWhenHintMisses(t *testing.T) {
	stub := &stubClassifier{choice: gemini.CategoryChoice{
		CategoryID: "c2", CategoryName: "Transport", Confidence: gemini.ConfidenceHigh,
	}}

	got, err := ResolveCategoryChoice(context.Background(), stub, "bensin motor", sampleCategories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if !stub.called {
		t.Error("classifier should have been consulted")
	}
	if got.CategoryID != "c2" || got.Confidence != gemini.ConfidenceHigh {
		t.Errorf("choice = %+v, want c2/high", got)
	}
}

// A low-confidence classification is discarded in favour of the fallback, and
// the result stays "low" so it can never auto-save.
func TestResolveCategoryChoice_LowConfidenceFallsBack(t *testing.T) {
	stub := &stubClassifier{choice: gemini.CategoryChoice{
		CategoryID: "c1", CategoryName: "Makanan", Confidence: gemini.ConfidenceLow,
	}}

	got, err := ResolveCategoryChoice(context.Background(), stub, "xyz tidak jelas", sampleCategories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.CategoryID != "c3" {
		t.Errorf("id = %s, want c3 (fallback Belanja)", got.CategoryID)
	}
	if got.Confidence != gemini.ConfidenceLow {
		t.Errorf("confidence = %s, want low", got.Confidence)
	}
}

func TestResolveCategoryChoice_ClassifierErrorFallsBack(t *testing.T) {
	stub := &stubClassifier{err: errors.New("gemini unavailable")}

	got, err := ResolveCategoryChoice(context.Background(), stub, "beli sesuatu", sampleCategories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice must not surface classifier errors: %v", err)
	}
	if got.Confidence != gemini.ConfidenceLow {
		t.Errorf("confidence = %s, want low", got.Confidence)
	}
}

// With no classifier configured the bot still answers, just never silently.
func TestResolveCategoryChoice_NilClassifierFallsBack(t *testing.T) {
	got, err := ResolveCategoryChoice(context.Background(), nil, "beli sesuatu", sampleCategories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.Confidence != gemini.ConfidenceLow {
		t.Errorf("confidence = %s, want low", got.Confidence)
	}
}

// An income-sounding description steers the fallback away from the expense
// shopping default.
func TestResolveCategoryChoice_IncomeWordsAvoidShoppingFallback(t *testing.T) {
	categories := []domain.Category{
		cat("c3", "Belanja", domain.TransactionTypeExpense),
		cat("c5", "Gaji", domain.TransactionTypeIncome),
	}

	got, err := ResolveCategoryChoice(context.Background(), nil, "gaji bulanan", categories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.CategoryID != "c5" {
		t.Errorf("id = %s, want c5 (Gaji)", got.CategoryID)
	}
}

// INHERITED QUIRK, pinned so a future "cleanup" does not change real data:
// priority 2 of getFallbackCategory (transactionService.ts:136) matches on name
// only, with no type check. So when an "Lainnya" category exists and happens to
// be an EXPENSE, an income transaction falls back to it and is recorded as an
// expense. Preferring the income type only happens at priority 3, which this
// branch never reaches.
func TestResolveCategoryChoice_IncomeFallsIntoExpenseOtherCategory(t *testing.T) {
	got, err := ResolveCategoryChoice(context.Background(), nil, "gaji bulanan", sampleCategories, "")
	if err != nil {
		t.Fatalf("ResolveCategoryChoice: %v", err)
	}
	if got.CategoryID != "c4" {
		t.Errorf("id = %s, want c4 (Lainnya, an EXPENSE category) — the legacy quirk", got.CategoryID)
	}
}

func TestResolveCategoryChoice_EmptyCategoriesIsAnError(t *testing.T) {
	_, err := ResolveCategoryChoice(context.Background(), nil, "apa saja", nil, "")
	if !errors.Is(err, ErrNoCategories) {
		t.Errorf("err = %v, want ErrNoCategories", err)
	}
}
