package datetime

import (
	"testing"
	"time"
)

// Helper to build a time in Jakarta for tests.
func jakartaDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, Location())
}

func TestResolveRange(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		ref       time.Time
		wantStart string
		wantEnd   string
		wantErr   bool
	}{
		// --- today ---
		{
			name:      "today",
			spec:      "today",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-15",
			wantEnd:   "2026-07-15",
		},

		// --- yesterday ---
		{
			name:      "yesterday",
			spec:      "yesterday",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-14",
			wantEnd:   "2026-07-14",
		},
		{
			name:      "yesterday_cross_month",
			spec:      "yesterday",
			ref:       jakartaDate(2026, 8, 1),
			wantStart: "2026-07-31",
			wantEnd:   "2026-07-31",
		},

		// --- this_week (Monday-start) ---
		{
			name:      "this_week_wednesday",
			spec:      "this_week",
			ref:       jakartaDate(2026, 7, 15), // Wednesday
			wantStart: "2026-07-13",             // Monday
			wantEnd:   "2026-07-15",
		},
		{
			name:      "this_week_monday",
			spec:      "this_week",
			ref:       jakartaDate(2026, 7, 13), // Monday
			wantStart: "2026-07-13",
			wantEnd:   "2026-07-13",
		},
		{
			name:      "this_week_sunday",
			spec:      "this_week",
			ref:       jakartaDate(2026, 7, 19), // Sunday
			wantStart: "2026-07-13",             // previous Monday
			wantEnd:   "2026-07-19",
		},

		// --- last_week (INHERITED: last 7 days including today, NOT previous calendar week) ---
		{
			name:      "last_week",
			spec:      "last_week",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-09", // 15 - 6
			wantEnd:   "2026-07-15",
		},

		// --- this_month ---
		{
			name:      "this_month_july",
			spec:      "this_month",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-01",
			wantEnd:   "2026-07-31",
		},
		{
			name:      "this_month_feb_non_leap",
			spec:      "this_month",
			ref:       jakartaDate(2026, 2, 10),
			wantStart: "2026-02-01",
			wantEnd:   "2026-02-28",
		},

		// --- last_month ---
		{
			name:      "last_month_normal",
			spec:      "last_month",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-06-01",
			wantEnd:   "2026-06-30",
		},
		{
			name:      "last_month_march31_regression",
			spec:      "last_month",
			ref:       jakartaDate(2026, 3, 31),
			wantStart: "2026-02-01",
			wantEnd:   "2026-02-28",
			// This is the critical regression test. The old JS code produced
			// start=2026-03-01, end=2026-02-28 (inverted, empty range) due
			// to setMonth overflow. Our Go implementation must produce the
			// correct February range.
		},
		{
			name:      "last_month_leap_year_feb",
			spec:      "last_month",
			ref:       jakartaDate(2028, 3, 15), // 2028 is a leap year
			wantStart: "2028-02-01",
			wantEnd:   "2028-02-29",
		},
		{
			name:      "last_month_january",
			spec:      "last_month",
			ref:       jakartaDate(2026, 1, 10),
			wantStart: "2025-12-01",
			wantEnd:   "2025-12-31",
		},

		// --- custom_month ---
		{
			name:      "custom_month_30_days",
			spec:      "custom_month:2026-06",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-06-01",
			wantEnd:   "2026-06-30",
		},
		{
			name:      "custom_month_31_days",
			spec:      "custom_month:2026-07",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-01",
			wantEnd:   "2026-07-31",
		},
		{
			name:      "custom_month_leap_feb",
			spec:      "custom_month:2028-02",
			ref:       jakartaDate(2028, 3, 1),
			wantStart: "2028-02-01",
			wantEnd:   "2028-02-29",
		},
		{
			name:      "custom_month_non_leap_feb",
			spec:      "custom_month:2026-02",
			ref:       jakartaDate(2026, 3, 1),
			wantStart: "2026-02-01",
			wantEnd:   "2026-02-28",
		},

		// --- days_ago ---
		{
			name:      "days_ago_0",
			spec:      "days_ago:0",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-15",
			wantEnd:   "2026-07-15",
		},
		{
			name:      "days_ago_3",
			spec:      "days_ago:3",
			ref:       jakartaDate(2026, 7, 15),
			wantStart: "2026-07-12",
			wantEnd:   "2026-07-12",
		},

		// --- errors ---
		{
			name:    "unknown_spec",
			spec:    "next_year",
			ref:     jakartaDate(2026, 7, 15),
			wantErr: true,
		},
		{
			name:    "custom_month_bad_format",
			spec:    "custom_month:2026",
			ref:     jakartaDate(2026, 7, 15),
			wantErr: true,
		},
		{
			name:    "days_ago_negative",
			spec:    "days_ago:-1",
			ref:     jakartaDate(2026, 7, 15),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := ResolveRange(tt.spec, tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got start=%q end=%q", start, end)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tt.wantStart {
				t.Errorf("start = %q, want %q", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %q, want %q", end, tt.wantEnd)
			}
		})
	}
}
