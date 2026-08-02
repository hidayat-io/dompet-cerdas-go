package transaction

import (
	"regexp"
	"strings"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/money"
)

var (
	queryKeywords = []string{
		"berapa", "total", "pengeluaran", "pemasukan", "saldo", "balance",
		"transaksi", "detail", "rincian", "kategori", "breakdown", "boros", "gimana",
		"bagaimana", "tips", "saran", "analisa", "analisis", "history", "riwayat", "daftar", "list",
	}

	QueryPrefixRegex          = regexp.MustCompile(`(?i)^(tampilkan|tampilin|tunjukkan|lihat(?:kan)?|liat(?:in)?|show|cek)\b`)
	TransactionQueryWordRegex = regexp.MustCompile(`(?i)\b(trans(?:aksi)?s?|transs|transaski|transaksi|transsaksi|tranaksi|transactions?|txs?)\b`)
	QueryTargetRegex          = regexp.MustCompile(`(?i)\b(trans(?:aksi)?s?|transs|transaski|transaksi|transsaksi|tranaksi|transactions?|txs?)\b|\b(riwayat|detail|rincian|pengeluaran|pemasukan|saldo|kategori|breakdown)\b`)
	QueryOrderOrTimeRegex     = regexp.MustCompile(`(?i)\b(terakhir|sekarang|hari\s+ini|minggu\s+ini|bulan\s+ini|bln\s+ini|bulan\s+ni|kemarin|hari\s+terakhir|last|latest)\b`)
	QueryRankingRegex         = regexp.MustCompile(`(?i)\b(top|biggest|highest|largest|tertinggi|terbesar|terbanyak)\b`)
	EntryPrefixRegex          = regexp.MustCompile(`(?i)^(tambah(?:in)?|catat(?:kan)?|input|masukin|masukan|record|log)\s+`)
	ExplicitEntryVerb         = regexp.MustCompile(`(?i)^(tambah(?:in)?|catat(?:kan)?|input|masukin|masukan)\b`)
	AmountRegex               = regexp.MustCompile(`(?i)(?:rp\s*|idr\s*)?\d[\d.,]*\s*(?:k|rb|ribu|jt|juta|m|milyar|miliar)?\b`)
	LocalMultiEntrySeparator  = regexp.MustCompile(`(?i)\s+(?:dan|lalu|terus|trus|&)\s+`)
)

func containsQueryKeywords(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range queryKeywords {
		// INHERITED BEHAVIOR: Substring matching is used instead of word boundaries.
		// "bayar listrik 350rb" gets rejected because "listrik" contains "list".
		// See ADR-013. We preserve this for exact bug-for-bug parity.
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func looksLikeQueryMessage(message string) bool {
	lower := strings.TrimSpace(strings.ToLower(message))
	if containsQueryKeywords(lower) {
		return true
	}
	if QueryPrefixRegex.MatchString(lower) && (QueryTargetRegex.MatchString(lower) || QueryOrderOrTimeRegex.MatchString(lower)) {
		return true
	}

	// Dynamic regex: \b\d+\s* + TRANSACTION_QUERY_WORD body + \s+terakhir\b
	if regexp.MustCompile(`(?i)\b\d+\s*(trans(?:aksi)?s?|transs|transaski|transaksi|transsaksi|tranaksi|transactions?|txs?)\s+terakhir\b`).MatchString(lower) {
		return true
	}

	if regexp.MustCompile(`(?i)\b(?:(?:last|latest)\s+\d+|\d+\s+(?:last|latest))\s*(trans(?:aksi)?s?|transs|transaski|transaksi|transsaksi|tranaksi|transactions?|txs?)\b`).MatchString(lower) {
		return true
	}

	if regexp.MustCompile(`(?i)\btop\s+\d+\b`).MatchString(lower) && TransactionQueryWordRegex.MatchString(lower) {
		return true
	}

	if QueryRankingRegex.MatchString(lower) && regexp.MustCompile(`\b\d+\b`).MatchString(lower) && TransactionQueryWordRegex.MatchString(lower) {
		return true
	}
	return false
}

// ShouldAttemptTransactionParsing guards the parser. False positives here
// lead to silent erroneous financial records.
func ShouldAttemptTransactionParsing(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "/") {
		return false
	}
	if strings.Contains(trimmed, "?") || strings.Contains(trimmed, "？") {
		return false
	}

	lower := strings.ToLower(trimmed)
	hasExplicitEntryVerb := ExplicitEntryVerb.MatchString(lower)

	if !hasExplicitEntryVerb && looksLikeQueryMessage(trimmed) {
		return false
	}

	if !AmountRegex.MatchString(trimmed) {
		return false
	}

	if containsQueryKeywords(trimmed) && !hasExplicitEntryVerb {
		return false
	}

	return true
}

// MaxParsedTransactionItems caps how many drafts a single message may produce,
// matching MAX_PARSED_TRANSACTION_ITEMS in transactionParsingService.ts.
//
// The legacy local parser does not apply this cap; only the LLM path slices to
// it. It is applied here as a defensive bound, which is safe because the local
// parser requires exactly one amount per segment and 20 segments is far beyond
// any real message.
const MaxParsedTransactionItems = 20

// ParseLocally parses a message using only regex, with no LLM call.
//
// It returns nil when any segment fails to yield a draft, which is how the
// caller learns to fall back to the LLM. Partial results are never returned: the
// legacy parser requires every segment to parse, otherwise a message like
// "makan 25rb, ngobrol" would silently save only the first half.
func ParseLocally(message string) *domain.HybridTransactionParseResult {
	segments := splitCandidateSegments(message)
	if len(segments) == 0 {
		return nil
	}

	items := make([]domain.ParsedTransactionDraft, 0, len(segments))
	for _, segment := range segments {
		item := extractManualTransactionLocal(segment)
		if item == nil {
			return nil
		}
		items = append(items, *item)
	}

	if len(items) == 0 {
		return nil
	}
	if len(items) > MaxParsedTransactionItems {
		items = items[:MaxParsedTransactionItems]
	}

	// Confidence gates auto-save: only a single-item parse is confident enough to
	// skip user confirmation (transactionParsingService.ts:271).
	confidence := domain.Confidence("medium")
	if len(items) == 1 {
		confidence = domain.Confidence("high")
	}

	return &domain.HybridTransactionParseResult{
		Items:      items,
		UsedAI:     false,
		Confidence: confidence,
	}
}

// splitCandidateSegments divides a message into per-transaction segments.
//
// Separator precedence is newline, then semicolon, then comma, then conjunction.
// Comma and conjunction only split when the message contains more than one
// amount, so "beli kopi, gula 25rb" stays a single transaction.
func splitCandidateSegments(message string) []string {
	normalized := strings.TrimSpace(message)
	if normalized == "" {
		return nil
	}

	amountCount := len(AmountRegex.FindAllString(normalized, -1))

	var raw []string
	switch {
	case strings.Contains(normalized, "\n"):
		raw = strings.Split(normalized, "\n")
	case strings.Contains(normalized, ";"):
		raw = strings.Split(normalized, ";")
	case amountCount > 1 && strings.Contains(normalized, ","):
		raw = strings.Split(normalized, ",")
	case amountCount > 1 && LocalMultiEntrySeparator.MatchString(normalized):
		raw = LocalMultiEntrySeparator.Split(normalized, -1)
	default:
		raw = []string{normalized}
	}

	segments := make([]string, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(EntryPrefixRegex.ReplaceAllString(s, ""))
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

func extractManualTransactionLocal(segment string) *domain.ParsedTransactionDraft {
	catKeywordRegex := regexp.MustCompile(`(?i)\b(cat|categ|category|kategori|kat|ktg|ktgr|kate)\b\s*[:\-]?\s*([a-zA-ZÀ-ÿ0-9]+(?:\s+[a-zA-ZÀ-ÿ0-9]+){0,2})`)
	var categoryHint string
	cleanedMessage := segment

	if m := catKeywordRegex.FindStringSubmatch(segment); len(m) > 0 {
		categoryHint = strings.TrimSpace(m[2])
		cleanedMessage = strings.Replace(cleanedMessage, m[0], " ", 1)
		cleanedMessage = regexp.MustCompile(`\s{2,}`).ReplaceAllString(cleanedMessage, " ")
		cleanedMessage = strings.TrimSpace(cleanedMessage)
	}

	matches := AmountRegex.FindAllString(cleanedMessage, -1)
	if len(matches) != 1 {
		return nil
	}

	amountToken := matches[0]
	amount, ok := money.ParseAmount(amountToken)
	if !ok || amount <= 0 {
		return nil
	}

	tokenRegex := regexp.MustCompile("(?i)" + regexp.QuoteMeta(amountToken))
	description := tokenRegex.ReplaceAllString(cleanedMessage, " ")
	description = regexp.MustCompile(`[\s:;\-–—]+$`).ReplaceAllString(description, "")
	description = strings.TrimSpace(description)

	if description == "" {
		return nil
	}

	lowerDesc := strings.ToLower(description)
	if categoryHint == "" {
		if regexp.MustCompile(`(?i)makan|sarapan|breakfast|lunch|dinner|warteg|warung|kopi|cafe|resto|restaurant|snack|camil|camilan|keripik|chips|roti|ayam|nasi|minum`).MatchString(lowerDesc) {
			categoryHint = "Food"
		} else if regexp.MustCompile(`(?i)grab|gojek|ojek|bus|kereta|taxi|tol|parkir|bensin|bbm|pertamina|transport`).MatchString(lowerDesc) {
			categoryHint = "Transportation"
		} else if regexp.MustCompile(`(?i)belanja|shopping|market|minimarket|indomaret|alfamart|supermarket|mall`).MatchString(lowerDesc) {
			categoryHint = "Shopping"
		} else if regexp.MustCompile(`(?i)tagihan|bill|bayar\s+hutang|bayar\s+utang|hutang|utang|cicilan|kredit|pinjaman`).MatchString(lowerDesc) {
			categoryHint = "Bill"
		} else if regexp.MustCompile(`(?i)gaji|salary|fee|bayaran|komisi|bonus|thr|income|pemasukan`).MatchString(lowerDesc) {
			categoryHint = "Income"
		}
	}

	return &domain.ParsedTransactionDraft{
		Amount:       amount,
		Description:  description,
		CategoryHint: categoryHint,
		SourceText:   strings.TrimSpace(segment),
	}
}

// NormalizeParsedTransactionDrafts sanitizes the results.
func NormalizeParsedTransactionDrafts(items []domain.ParsedTransactionDraft) []domain.ParsedTransactionDraft {
	var valid []domain.ParsedTransactionDraft
	for _, it := range items {
		if it.Amount > 0 && strings.TrimSpace(it.Description) != "" {

			valid = append(valid, it)
		}
	}
	return valid
}

// ShouldAutoSave is the auto-save gate.
// CRITICAL: This is the most dangerous path. False positives silently write wrong financial records.
// SEE ADR-011: Audit logging is required in Go.
func ShouldAutoSave(result *domain.HybridTransactionParseResult, categoryResolvedByAI bool) bool {
	if result == nil {
		return false
	}
	return len(result.Items) == 1 && !result.UsedAI && !categoryResolvedByAI
}
