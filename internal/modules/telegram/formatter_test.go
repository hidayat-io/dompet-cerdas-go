package telegram

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

// TestEscapeMarkdown_Fixtures replays escapeMarkdown output captured from the
// legacy backend.
//
// Production sends messages with parse_mode "Markdown" (V1). The escape set is
// the MarkdownV2 character class, which is broader than V1 needs, but that is
// live behavior and switching to V2 would change how every bot message renders.
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
		{name: "int64", input: int64(-50000), want: `\-50000`},
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

func TestWithAccountHeader(t *testing.T) {
	t.Run("empty_account_returns_message_unchanged", func(t *testing.T) {
		msg := "Saldo kamu: Rp 1.000"
		if got := WithAccountHeader(msg, ""); got != msg {
			t.Errorf("WithAccountHeader(msg, \"\") = %q, want the message unchanged", got)
		}
	})

	t.Run("account_name_is_escaped", func(t *testing.T) {
		got := WithAccountHeader("body", "Dompet.Utama")
		want := "📁 *Akun: Dompet\\.Utama*\n\nbody"
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

// An unparseable date must be escaped rather than passed through raw, otherwise
// it could break Telegram's Markdown parser.
func TestFormatDate_InvalidInputIsEscaped(t *testing.T) {
	got := FormatDate("not-a-date")
	if got == "not-a-date" {
		t.Errorf("FormatDate(%q) = %q, want the value escaped", "not-a-date", got)
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
			wantStatus: "Saldo positif", wantExactRp: "Rp 1\\.500\\.000",
		},
		{
			name: "zero", balance: 0, wantEmoji: "ℹ️",
			wantStatus: "Saldo nol", wantExactRp: "Rp 0",
		},
		{
			name: "negative", balance: -50_000, wantEmoji: "⚠️",
			wantStatus: "Saldo negatif", wantExactRp: "Rp \\-50\\.000",
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
