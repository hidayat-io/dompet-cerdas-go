// Package money provides Indonesian Rupiah (IDR) amount parsing and formatting.
// It ports the parsing and formatting logic from the old TypeScript backend,
// preserving all production-load-bearing quirks documented inline.
package money

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// suffixMultipliers maps Indonesian amount suffixes to their multiplier value.
// Case-insensitive matching is performed before lookup.
var suffixMultipliers = map[string]float64{
	"k":      1_000,
	"rb":     1_000,
	"ribu":   1_000,
	"jt":     1_000_000,
	"juta":   1_000_000,
	"m":      1_000_000_000,
	"milyar": 1_000_000_000,
	"miliar": 1_000_000_000,
}

// suffixRegex matches a trailing Indonesian multiplier suffix.
var suffixRegex = regexp.MustCompile(`(?i)(milyar|miliar|ribu|juta|rb|jt|k|m)$`)

// prefixRegex strips common Indonesian currency prefixes.
var prefixRegex = regexp.MustCompile(`(?i)^(rp|idr)\s*`)

// ParseAmount parses an Indonesian amount string into an integer (IDR, no decimals).
// It handles:
//   - Currency prefixes: "rp", "idr" (case-insensitive, stripped)
//   - Multiplier suffixes: k/rb/ribu (×1000), jt/juta (×1M), m/milyar/miliar (×1B)
//   - Number normalization (see NormalizeNumber for details)
//
// Returns 0 and false if the input cannot be parsed.
func ParseAmount(s string) (int64, bool) {
	cleaned := strings.TrimSpace(s)
	cleaned = strings.ToLower(cleaned)
	cleaned = prefixRegex.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return 0, false
	}

	// Detect suffix.
	suffixMatch := suffixRegex.FindString(cleaned)
	suffix := strings.ToLower(suffixMatch)
	numberPart := cleaned
	if suffix != "" {
		numberPart = strings.TrimSpace(cleaned[:len(cleaned)-len(suffix)])
	}

	if numberPart == "" {
		return 0, false
	}

	hasSuffix := suffix != ""
	baseValue, ok := normalizeNumber(numberPart, hasSuffix)
	if !ok {
		return 0, false
	}

	multiplier := 1.0
	if m, exists := suffixMultipliers[suffix]; exists {
		multiplier = m
	}

	result := math.Round(baseValue * multiplier)
	return int64(result), true
}

// normalizeNumber parses a numeric string with Indonesian formatting conventions.
//
// WITH a suffix present: commas are treated as decimal separators.
// Replace ',' with '.' then parse as float. So "2,5" → 2.5.
//
// WITHOUT a suffix: strip ALL '.' and ',' then parse as integer.
// So "150.000" → 150000 and "20,000" → 20000.
//
// KNOWN WART (INHERITED BEHAVIOR): Without a suffix, "150.50" becomes 15050
// because dots are stripped as thousands separators. This is preserved intentionally
// to maintain production behavior parity with the old TypeScript backend.
func normalizeNumber(raw string, hasSuffix bool) (float64, bool) {
	if raw == "" {
		return 0, false
	}

	if hasSuffix {
		// With suffix: comma → decimal dot.
		normalized := strings.ReplaceAll(raw, ",", ".")
		val, err := strconv.ParseFloat(normalized, 64)
		if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, false
		}
		return val, true
	}

	// Without suffix: strip all separators and parse as integer.
	digitsOnly := strings.NewReplacer(".", "", ",", "").Replace(raw)
	val, err := strconv.ParseInt(digitsOnly, 10, 64)
	if err != nil {
		return 0, false
	}
	return float64(val), true
}

// FormatCompact formats an IDR amount in a human-friendly compact form,
// used in Telegram bot replies. It mirrors formatRupiah() in the old
// TypeScript backend (functions/src/services/responseFormatter.ts:284):
//
//	if (amount >= 1_000_000) return `Rp ${(amount/1_000_000).toFixed(1)}jt`
//	else if (amount >= 1_000) return `Rp ${(amount/1_000).toFixed(0)}k`
//	else                      return `Rp ${amount.toLocaleString('id-ID')}`
//
// Two inherited quirks are verified against the old runtime and preserved:
//
// Rounding, not truncation. toFixed() rounds, so 25_600 → "Rp 26k" and
// 999_999 → "Rp 1000k" (not "Rp 999k"). math.Round reproduces this.
//
// Negative amounts always fall through to the final branch, because the
// comparisons are on the signed value: -50_000 >= 1_000 is false. So
// -50_000 → "Rp -50.000", never "-Rp 50k".
func FormatCompact(amount int64) string {
	switch {
	case amount >= 1_000_000:
		val := math.Round(float64(amount)/1_000_000.0*10) / 10
		return fmt.Sprintf("Rp %.1fjt", val)
	case amount >= 1_000:
		val := int64(math.Round(float64(amount) / 1_000.0))
		return fmt.Sprintf("Rp %dk", val)
	default:
		return "Rp " + groupThousands(amount)
	}
}

// FormatExact formats an IDR amount with full Indonesian dot grouping,
// mirroring formatExactRupiah() (functions/src/services/responseFormatter.ts:297):
//
//	`Rp ${amount.toLocaleString('id-ID')}`
//
// The sign sits after the "Rp " literal, so -2_500_000 → "Rp -2.500.000".
func FormatExact(amount int64) string {
	return "Rp " + groupThousands(amount)
}

// groupThousands renders an integer with Indonesian dot grouping, matching
// Number.prototype.toLocaleString('id-ID'). The minus sign stays attached to
// the digits: -2_500_000 → "-2.500.000".
func groupThousands(amount int64) string {
	neg := amount < 0
	abs := amount
	if neg {
		abs = -abs
	}

	s := strconv.FormatInt(abs, 10)
	n := len(s)

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}

	if n <= 3 {
		b.WriteString(s)
		return b.String()
	}

	remainder := n % 3
	wroteDigits := false
	if remainder > 0 {
		b.WriteString(s[:remainder])
		wroteDigits = true
	}
	for i := remainder; i < n; i += 3 {
		if wroteDigits {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
		wroteDigits = true
	}

	return b.String()
}
