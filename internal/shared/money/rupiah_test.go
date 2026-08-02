package money

import (
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   int64
		wantOK bool
	}{
		// --- Suffix: k / rb / ribu (×1000) ---
		{name: "k_suffix", input: "25k", want: 25_000, wantOK: true},
		{name: "rb_suffix", input: "25rb", want: 25_000, wantOK: true},
		{name: "ribu_suffix", input: "25ribu", want: 25_000, wantOK: true},
		{name: "K_uppercase", input: "25K", want: 25_000, wantOK: true},
		{name: "ribu_decimal_comma", input: "2,5ribu", want: 2_500, wantOK: true},

		// --- Suffix: jt / juta (×1_000_000) ---
		{name: "jt_suffix", input: "2jt", want: 2_000_000, wantOK: true},
		{name: "juta_suffix", input: "2juta", want: 2_000_000, wantOK: true},
		{name: "jt_decimal_comma", input: "2,5jt", want: 2_500_000, wantOK: true},
		{name: "jt_decimal_dot", input: "1.5jt", want: 1_500_000, wantOK: true},

		// --- Suffix: m / milyar / miliar (×1_000_000_000) ---
		{name: "m_suffix", input: "1m", want: 1_000_000_000, wantOK: true},
		{name: "milyar_suffix", input: "1milyar", want: 1_000_000_000, wantOK: true},
		{name: "miliar_suffix", input: "1miliar", want: 1_000_000_000, wantOK: true},
		{name: "miliar_decimal", input: "2,5miliar", want: 2_500_000_000, wantOK: true},

		// --- No suffix: strip . and , as thousands separators ---
		{name: "plain_number", input: "50000", want: 50_000, wantOK: true},
		{name: "dot_thousands", input: "150.000", want: 150_000, wantOK: true},
		{name: "comma_thousands", input: "20,000", want: 20_000, wantOK: true},
		{name: "multiple_dots", input: "1.500.000", want: 1_500_000, wantOK: true},

		// --- Known wart: without suffix, "150.50" becomes 15050 ---
		// INHERITED BEHAVIOR: Dots are stripped as thousands separators when there is
		// no suffix. This means "150.50" is treated as "15050" not "150.50".
		// Preserved intentionally for production behavior parity.
		{name: "wart_dot_without_suffix", input: "150.50", want: 15050, wantOK: true},

		// --- Currency prefix stripping ---
		{name: "rp_prefix", input: "Rp 25000", want: 25_000, wantOK: true},
		{name: "rp_prefix_no_space", input: "rp25k", want: 25_000, wantOK: true},
		{name: "idr_prefix", input: "IDR 50.000", want: 50_000, wantOK: true},

		// --- Edge cases ---
		{name: "empty_string", input: "", want: 0, wantOK: false},
		{name: "only_suffix", input: "jt", want: 0, wantOK: false},
		{name: "not_a_number", input: "abc", want: 0, wantOK: false},
		{name: "small_number", input: "500", want: 500, wantOK: true},

		// --- Space between number and suffix ---
		{name: "space_before_suffix", input: "25 rb", want: 25_000, wantOK: true},
		{name: "rp_prefix_dotted_millions", input: "rp 1.250.000", want: 1_250_000, wantOK: true},
		{name: "m_five_billion", input: "5m", want: 5_000_000_000, wantOK: true},
		{name: "juta_comma_decimal", input: "1,25juta", want: 1_250_000, wantOK: true},
		{name: "jt_leading_zero_decimal", input: "0,5jt", want: 500_000, wantOK: true},
		{name: "jt_dot_decimal", input: "2.5jt", want: 2_500_000, wantOK: true},

		// --- FormatCompact boundary values ---
		{name: "boundary_999", input: "999", want: 999, wantOK: true},
		{name: "boundary_1000", input: "1000", want: 1_000, wantOK: true},
		{name: "boundary_999999", input: "999999", want: 999_999, wantOK: true},
		{name: "boundary_1000000", input: "1000000", want: 1_000_000, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseAmount(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("ParseAmount(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("ParseAmount(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// Expected values were captured by executing the old TypeScript implementation
// (functions/src/services/responseFormatter.ts formatRupiah) under Node, so this
// table is a parity contract rather than a guess at intended behavior.
func TestFormatCompact(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   string
	}{
		{name: "below_1k", amount: 500, want: "Rp 500"},
		{name: "exact_999", amount: 999, want: "Rp 999"},
		{name: "exact_1000", amount: 1_000, want: "Rp 1k"},
		{name: "25k", amount: 25_000, want: "Rp 25k"},
		{name: "rounds_up_not_truncates", amount: 25_600, want: "Rp 26k"},
		{name: "999999_rounds_to_1000k", amount: 999_999, want: "Rp 1000k"},
		{name: "exact_1M", amount: 1_000_000, want: "Rp 1.0jt"},
		{name: "2_5M", amount: 2_500_000, want: "Rp 2.5jt"},
		{name: "1049999_rounds_to_1_0jt", amount: 1_049_999, want: "Rp 1.0jt"},
		{name: "zero", amount: 0, want: "Rp 0"},
		{name: "negative_falls_through_to_exact", amount: -50_000, want: "Rp -50.000"},
		{name: "negative_million_falls_through", amount: -2_500_000, want: "Rp -2.500.000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCompact(tt.amount)
			if got != tt.want {
				t.Errorf("FormatCompact(%d) = %q, want %q", tt.amount, got, tt.want)
			}
		})
	}
}

func TestFormatExact(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   string
	}{
		{name: "small", amount: 500, want: "Rp 500"},
		{name: "thousands", amount: 25_000, want: "Rp 25.000"},
		{name: "millions", amount: 2_500_000, want: "Rp 2.500.000"},
		{name: "billions", amount: 1_000_000_000, want: "Rp 1.000.000.000"},
		{name: "zero", amount: 0, want: "Rp 0"},
		{name: "negative", amount: -150_000, want: "Rp -150.000"},
		{name: "exact_1000", amount: 1_000, want: "Rp 1.000"},
		{name: "exact_1M", amount: 1_000_000, want: "Rp 1.000.000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatExact(tt.amount)
			if got != tt.want {
				t.Errorf("FormatExact(%d) = %q, want %q", tt.amount, got, tt.want)
			}
		})
	}
}

func TestParseAmount_Fixtures(t *testing.T) {
	const fixture = "amount_parse.json"

	var data struct {
		Cases []struct {
			Input  string   `json:"input"`
			Output *float64 `json:"output"`
		} `json:"cases"`
	}
	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		got, ok := ParseAmount(c.Input)

		if c.Output == nil {
			if ok {
				t.Errorf("ParseAmount(%q) = (%d, true), want ok=false", c.Input, got)
			}
			continue
		}

		want := int64(*c.Output)
		if !ok {
			t.Errorf("ParseAmount(%q) = ok=false, want (%d, true)", c.Input, want)
			continue
		}
		if got != want {
			t.Errorf("ParseAmount(%q) = %d, want %d", c.Input, got, want)
		}
	}
}

func TestFormatRupiah_Fixtures(t *testing.T) {
	const fixture = "money_format.json"

	var data struct {
		Cases []struct {
			Amount  int64  `json:"amount"`
			Compact string `json:"compact"`
			Exact   string `json:"exact"`
		} `json:"cases"`
	}
	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for _, c := range data.Cases {
		if got := FormatCompact(c.Amount); got != c.Compact {
			t.Errorf("FormatCompact(%d) = %q, want %q", c.Amount, got, c.Compact)
		}
		if got := FormatExact(c.Amount); got != c.Exact {
			t.Errorf("FormatExact(%d) = %q, want %q", c.Amount, got, c.Exact)
		}
	}
}
