package telegram

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

// TestEscapeMarkdown_Fixtures pins EscapeMarkdown's output against
// testdata/parity/markdown_escape.json.
//
// This fixture no longer replays the legacy backend byte-for-byte: the legacy
// escaper used the MarkdownV2 character class while every send call (legacy
// and Go alike) uses parse_mode "Markdown" (V1), so it escaped characters V1
// never treats as special and leaked a raw backslash in front of them (e.g.
// "Rp150\.000" shown to the user). This is documented as a deliberate
// divergence in docs/PARITY_CONTRACT.md section 5 and docs/DECISIONS.md.
func TestEscapeMarkdown_Fixtures(t *testing.T) {
	const fixture = "markdown_escape.json"

	var data struct {
		Cases []struct {
			Input             interface{} `json:"input"`
			InputWasUndefined bool        `json:"inputWasUndefined"`
			Output            string      `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		input := c.Input
		if c.InputWasUndefined {
			input = nil
		}

		if got := EscapeMarkdown(input); got != c.Output {
			t.Errorf("EscapeMarkdown(%#v) = %q, want %q", c.Input, got, c.Output)
		}
	}
}

// JSON decoding turns numbers into float64, so the fixture cannot prove that an
// int input renders without a decimal point. These cases cover it directly.
func TestEscapeMarkdown_NonStringInputs(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "empty_string", input: "", want: ""},
		{name: "int", input: 25000, want: "25000"},
		{name: "int64", input: int64(-50000), want: "-50000"},
		{name: "plain_text", input: "no special chars", want: "no special chars"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeMarkdown(tt.input); got != tt.want {
				t.Errorf("EscapeMarkdown(%#v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestEscapeMarkdown_V1DoesNotOverEscape locks the fix for a bug where
// messages rendered with a literal backslash in front of every period,
// exclamation mark, dash, etc. (e.g. "Rp150\.000") because EscapeMarkdown
// escaped the MarkdownV2 character class while every send call uses
// parse_mode "Markdown" (V1). V1 only recognizes '_', '*', '`', '[' as
// escapable, so escaping anything else just leaks a raw backslash into the
// rendered text.
func TestEscapeMarkdown_V1DoesNotOverEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "period_not_escaped", input: "Rp150.000", want: "Rp150.000"},
		{name: "exclamation_not_escaped", input: "Wow!", want: "Wow!"},
		{name: "dash_not_escaped", input: "Transfer - DP rumah", want: "Transfer - DP rumah"},
		{name: "parens_not_escaped", input: "Toko (Cabang 2)", want: "Toko (Cabang 2)"},
		{name: "underscore_still_escaped", input: "_italic_", want: `\_italic\_`},
		{name: "asterisk_still_escaped", input: "*bold*", want: `\*bold\*`},
		{name: "backtick_still_escaped", input: "`code`", want: "\\`code\\`"},
		{name: "open_bracket_still_escaped", input: "[link]", want: `\[link]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeMarkdown(tt.input); got != tt.want {
				t.Errorf("EscapeMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithAccountHeader(t *testing.T) {
	t.Run("empty_account_returns_message_unchanged", func(t *testing.T) {
		msg := "Saldo kamu: Rp 1.000"
		if got := WithAccountHeader(msg, ""); got != msg {
			t.Errorf("WithAccountHeader(msg, \"\") = %q, want the message unchanged", got)
		}
	})

	t.Run("account_name_is_escaped", func(t *testing.T) {
		got := WithAccountHeader("body", "Dompet.Utama")
		want := "📁 *Akun: Dompet.Utama*\n\nbody"
		if got != want {
			t.Errorf("WithAccountHeader() = %q, want %q", got, want)
		}
	})
}

// The expectations below were captured by running the legacy formatDate against
// the same inputs under Asia/Jakarta. An earlier revision of this test omitted
// the weekday suffix, having trusted the doc comment in responseFormatter.ts
// ("e.g., '27 Jan'") rather than the line beneath it, which appends " - <hari>".
func TestFormatDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "january", input: "2026-01-27", want: "27 Jan - Selasa"},
		{name: "may_uses_indonesian_mei", input: "2026-05-01", want: "1 Mei - Jumat"},
		{name: "august_uses_agt", input: "2026-08-15", want: "15 Agt - Sabtu"},
		{name: "october_uses_okt", input: "2026-10-09", want: "9 Okt - Jumat"},
		{name: "december_uses_des", input: "2026-12-31", want: "31 Des - Kamis"},
		{name: "empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDate(tt.input); got != tt.want {
				t.Errorf("FormatDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// An unparseable date must go through EscapeMarkdown rather than being passed
// through raw, otherwise a stray '_', '*', '`' or '[' in it could break
// Telegram's Markdown parser. The probe uses '_' and '*' specifically because
// those are the characters V1 Markdown actually treats as special — a plain
// dash (V1-safe) wouldn't demonstrate escaping happened.
func TestFormatDate_InvalidInputIsEscaped(t *testing.T) {
	got := FormatDate("not_a*date")
	want := `not\_a\*date`
	if got != want {
		t.Errorf("FormatDate(%q) = %q, want %q", "not_a*date", got, want)
	}
}

func TestFormatBalanceResponse(t *testing.T) {
	tests := []struct {
		name        string
		balance     int64
		timeRange   string
		wantEmoji   string
		wantStatus  string
		wantExactRp string
	}{
		{
			name: "positive", balance: 1_500_000, wantEmoji: "💰",
			wantStatus: "Saldo positif", wantExactRp: "Rp 1.500.000",
		},
		{
			name: "zero", balance: 0, wantEmoji: "ℹ️",
			wantStatus: "Saldo nol", wantExactRp: "Rp 0",
		},
		{
			name: "negative", balance: -50_000, wantEmoji: "⚠️",
			wantStatus: "Saldo negatif", wantExactRp: "Rp -50.000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBalanceResponse(tt.balance, tt.timeRange)
			if !contains(got, tt.wantEmoji) {
				t.Errorf("FormatBalanceResponse(%d) = %q, want it to contain %q", tt.balance, got, tt.wantEmoji)
			}
			if !contains(got, tt.wantStatus) {
				t.Errorf("FormatBalanceResponse(%d) = %q, want it to contain %q", tt.balance, got, tt.wantStatus)
			}
			if !contains(got, tt.wantExactRp) {
				t.Errorf("FormatBalanceResponse(%d) = %q, want it to contain %q", tt.balance, got, tt.wantExactRp)
			}
		})
	}
}

func TestFormatExpenseResponse_IncludesAverage(t *testing.T) {
	got := FormatExpenseResponse(100_000, 4, "bulan ini")
	if !contains(got, "4 transaksi") {
		t.Errorf("expected the transaction count in %q", got)
	}
	if !contains(got, "Rp 25k") {
		t.Errorf("expected the compact average Rp 25k in %q", got)
	}
}

// A zero count must not divide by zero.
func TestFormatExpenseResponse_ZeroCount(t *testing.T) {
	got := FormatExpenseResponse(0, 0, "hari ini")
	if !contains(got, "0 transaksi") {
		t.Errorf("expected a zero count in %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
