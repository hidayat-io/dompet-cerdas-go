package transaction

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
)

// ErrNoCategories is returned when an account has no categories to choose from.
var ErrNoCategories = errors.New("no categories available")

// Classifier is the LLM leg of category resolution. It is an interface so the
// deterministic legs can be tested without a network call, and so a nil
// classifier degrades to the fallback instead of panicking.
type Classifier interface {
	ClassifyCategory(ctx context.Context, description string, categories []gemini.CategoryCandidate) (gemini.CategoryChoice, error)
}

// CategoryChoice is a resolved category plus how much the bot trusts it.
//
// Confidence is not cosmetic: ShouldAutoSave requires "high", so anything that
// downgrades it turns a silent save into a confirmation prompt.
type CategoryChoice struct {
	CategoryID   string
	CategoryName string
	Confidence   string
}

// hintAliasMap expands a parsed category hint into the words a category name
// might contain, ported from resolveCategoryChoice (bot/index.ts:267).
var hintAliasMap = map[string][]string{
	"food":           {"makan", "makanan", "kuliner", "food", "minum", "snack", "camil", "camilan", "warteg", "warung", "resto", "restaurant", "cafe", "kopi"},
	"transportation": {"transport", "transportasi", "travel", "perjalanan", "bensin", "bbm", "parkir", "taksi", "ojek", "gojek", "grab", "pertamina"},
	"shopping":       {"belanja", "shopping", "market", "minimarket", "indomaret", "alfamart", "supermarket", "mall"},
	"bill":           {"tagihan", "bill", "hutang", "utang", "cicilan", "kredit", "pinjaman", "bayar hutang", "bayar utang"},
	"income":         {"gaji", "salary", "bonus", "thr", "fee", "komisi", "bayaran", "income", "pemasukan"},
}

// incomeWordsPattern decides the fallback's preferred type when classification
// fails, ported from the inline regex at bot/index.ts:306.
var incomeWordsPattern = regexp.MustCompile(`(?i)gaji|salary|bonus|thr|fee|bayaran|komisi|income|pemasukan`)

// belanjaNames, otherNames are the preferred fallback category names, ported
// from getFallbackCategory (transactionService.ts:120).
var (
	belanjaNames = map[string]bool{"belanja": true, "shopping": true, "belanja harian": true}
	otherNames   = map[string]bool{"lainnya": true, "other": true, "others": true}
)

// FallbackCategory picks a category when nothing matched, in the legacy order:
// a shopping category for expenses, then an "other" category of any type, then
// the first category of the preferred type, then simply the first one.
func FallbackCategory(categories []domain.Category, preferred domain.TransactionType) (domain.Category, error) {
	if len(categories) == 0 {
		return domain.Category{}, ErrNoCategories
	}

	if preferred == domain.TransactionTypeExpense {
		for _, cat := range categories {
			if cat.Type == domain.TransactionTypeExpense && belanjaNames[strings.ToLower(cat.Name)] {
				return cat, nil
			}
		}
	}

	for _, cat := range categories {
		if otherNames[strings.ToLower(cat.Name)] {
			return cat, nil
		}
	}

	for _, cat := range categories {
		if cat.Type == preferred {
			return cat, nil
		}
	}

	return categories[0], nil
}

// ResolveCategoryChoice maps a parsed transaction onto a category, porting
// resolveCategoryChoice (bot/index.ts:253).
//
// Order: exact hint match, then alias/substring match, then the classifier,
// then the fallback. The first two return "high" and are what make auto-save
// possible without an LLM round trip; the fallback always returns "low", so a
// guess is never saved without the user seeing it first.
func ResolveCategoryChoice(
	ctx context.Context,
	classifier Classifier,
	description string,
	categories []domain.Category,
	categoryHint string,
) (CategoryChoice, error) {
	if len(categories) == 0 {
		return CategoryChoice{}, ErrNoCategories
	}

	hint := strings.ToLower(strings.TrimSpace(categoryHint))

	if hint != "" {
		for _, cat := range categories {
			if strings.ToLower(cat.Name) == hint {
				return CategoryChoice{cat.ID, cat.Name, gemini.ConfidenceHigh}, nil
			}
		}

		aliases := hintAliasMap[hint]
		for _, cat := range categories {
			name := strings.ToLower(cat.Name)
			if strings.Contains(name, hint) || strings.Contains(hint, name) {
				return CategoryChoice{cat.ID, cat.Name, gemini.ConfidenceHigh}, nil
			}
			for _, alias := range aliases {
				if strings.Contains(name, alias) {
					return CategoryChoice{cat.ID, cat.Name, gemini.ConfidenceHigh}, nil
				}
			}
		}
	}

	if classifier != nil {
		choice, err := classifier.ClassifyCategory(ctx, description, toCandidates(categories))
		if err == nil && choice.Confidence != gemini.ConfidenceLow {
			return CategoryChoice{choice.CategoryID, choice.CategoryName, choice.Confidence}, nil
		}
	}

	preferred := domain.TransactionTypeExpense
	if hint == "income" || incomeWordsPattern.MatchString(description) {
		preferred = domain.TransactionTypeIncome
	}

	cat, err := FallbackCategory(categories, preferred)
	if err != nil {
		return CategoryChoice{}, err
	}
	return CategoryChoice{cat.ID, cat.Name, gemini.ConfidenceLow}, nil
}

func toCandidates(categories []domain.Category) []gemini.CategoryCandidate {
	out := make([]gemini.CategoryCandidate, 0, len(categories))
	for _, cat := range categories {
		out = append(out, gemini.CategoryCandidate{ID: cat.ID, Name: cat.Name, Type: string(cat.Type)})
	}
	return out
}

// HintAliases exposes the alias list for a hint so callers can tell a
// deterministic match from one that needed the classifier, which is the
// distinction the auto-save gate turns on (ADR-011).
func HintAliases(hint string) []string {
	return hintAliasMap[strings.ToLower(strings.TrimSpace(hint))]
}
