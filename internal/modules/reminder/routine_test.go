package reminder

import (
	"testing"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

func TestHourLabel(t *testing.T) {
	// 23:05 UTC is 06:05 the next day in Jakarta, so the label must come from
	// the Jakarta clock, not the incoming location.
	utc := time.Date(2026, 3, 9, 23, 5, 0, 0, time.UTC)

	if got := HourLabel(utc); got != "06:00" {
		t.Errorf("HourLabel = %q, want 06:00", got)
	}
}

func TestMonthKey(t *testing.T) {
	if got := MonthKey(time.Date(2026, 2, 9, 10, 0, 0, 0, datetime.Location())); got != "2026-02" {
		t.Errorf("MonthKey = %q, want 2026-02", got)
	}
}

func TestShouldRemindToday(t *testing.T) {
	tests := []struct {
		name           string
		exp            routineExpense
		day            int
		isStartOfMonth bool
		isEndOfMonth   bool
		hour           string
		want           bool
	}{
		{
			name: "start_of_month_matches",
			exp:  routineExpense{ReminderType: ReminderTypeStartOfMonth, ReminderTime: "08:00"},
			day:  1, isStartOfMonth: true, hour: "08:00", want: true,
		},
		{
			name: "start_of_month_wrong_day",
			exp:  routineExpense{ReminderType: ReminderTypeStartOfMonth, ReminderTime: "08:00"},
			day:  5, hour: "08:00", want: false,
		},
		{
			name: "end_of_month_matches",
			exp:  routineExpense{ReminderType: ReminderTypeEndOfMonth, ReminderTime: "20:00"},
			day:  31, isEndOfMonth: true, hour: "20:00", want: true,
		},
		{
			name: "custom_exact_day",
			exp:  routineExpense{ReminderType: ReminderTypeCustom, ReminderDate: 15, ReminderTime: "09:00"},
			day:  15, hour: "09:00", want: true,
		},
		{
			name: "custom_wrong_day",
			exp:  routineExpense{ReminderType: ReminderTypeCustom, ReminderDate: 15, ReminderTime: "09:00"},
			day:  14, hour: "09:00", want: false,
		},
		{
			name: "right_day_wrong_hour",
			exp:  routineExpense{ReminderType: ReminderTypeCustom, ReminderDate: 15, ReminderTime: "09:00"},
			day:  15, hour: "10:00", want: false,
		},
		{
			name: "missing_time_defaults_to_0800",
			exp:  routineExpense{ReminderType: ReminderTypeCustom, ReminderDate: 15},
			day:  15, hour: DefaultReminderTime, want: true,
		},
		{
			name: "unknown_reminder_type_never_fires",
			exp:  routineExpense{ReminderType: "SETIAP_JUMAT", ReminderTime: "08:00"},
			day:  1, isStartOfMonth: true, hour: "08:00", want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldRemindToday(tt.exp, tt.day, tt.isStartOfMonth, tt.isEndOfMonth, tt.hour)
			if got != tt.want {
				t.Errorf("ShouldRemindToday = %v, want %v", got, tt.want)
			}
		})
	}
}

// An expense due on the 31st must still fire in a shorter month, otherwise it
// would be skipped every February.
func TestShouldRemindToday_CustomDateBeyondMonthEndFiresOnLastDay(t *testing.T) {
	exp := routineExpense{ReminderType: ReminderTypeCustom, ReminderDate: 31, ReminderTime: "08:00"}

	if !ShouldRemindToday(exp, 28, false, true, "08:00") {
		t.Error("a custom date past the month end must fire on the last day")
	}
	// Not the last day yet: it must stay quiet.
	if ShouldRemindToday(exp, 27, false, false, "08:00") {
		t.Error("must not fire before the last day")
	}
}

func TestFormatRoutineExpenseReminder(t *testing.T) {
	got := FormatRoutineExpenseReminder("Listrik PLN", 350000)

	for _, want := range []string{"🔔 *Pengingat Pengeluaran Rutin*", "📋 *Listrik PLN*", "💰 *Rp 350.000*"} {
		if !contains(got, want) {
			t.Errorf("message missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestUserIDFromLinkPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "projects/p/databases/(default)/documents/users/abc123/telegram_link/main", want: "abc123"},
		{path: "users/xyz/telegram_link/main", want: "xyz"},
		{path: "something/else", want: ""},
	}

	for _, tt := range tests {
		if got := userIDFromLinkPath(tt.path); got != tt.want {
			t.Errorf("userIDFromLinkPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
