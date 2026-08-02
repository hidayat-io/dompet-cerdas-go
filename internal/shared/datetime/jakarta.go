// Package datetime provides Asia/Jakarta timezone helpers and date range
// resolution for the DompetCerdas finance app. It loads the timezone once
// at init and uses real calendar arithmetic — the old TypeScript backend
// added a raw 7-hour UTC offset which was fragile and DST-unsafe.
package datetime

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// jakartaLoc is the Asia/Jakarta timezone, loaded once at package init.
var jakartaLoc *time.Location

func init() {
	var err error
	jakartaLoc, err = time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// This should never happen with tzdata embedded or installed.
		panic(fmt.Sprintf("datetime: failed to load Asia/Jakarta timezone: %v", err))
	}
}

// Location returns the Asia/Jakarta *time.Location.
func Location() *time.Location {
	return jakartaLoc
}

// Now returns the current time in the Asia/Jakarta timezone.
func Now() time.Time {
	return time.Now().In(jakartaLoc)
}

// TodayString returns today's date in Asia/Jakarta as "YYYY-MM-DD".
func TodayString() string {
	return Now().Format("2006-01-02")
}

// ResolveRange converts a named date specification into a start and end date
// string pair ("YYYY-MM-DD"), resolved relative to ref (which should be in
// Asia/Jakarta). The supported specs mirror the old TypeScript backend's
// behavior — see inline comments for inherited quirks.
//
// Supported specs:
//   - "today"               → start = end = ref date
//   - "yesterday"           → start = end = ref date - 1
//   - "this_week"           → Monday of ref's week through ref date
//   - "last_week"           → ref-6 days through ref date (see note below)
//   - "this_month"          → 1st through last day of ref's month
//   - "last_month"          → 1st through last day of the previous month
//   - "custom_month:YYYY-MM" → 1st through last day of the given month
//   - "days_ago:N"          → a single day N days before ref (start == end)
func ResolveRange(spec string, ref time.Time) (start, end string, err error) {
	ref = ref.In(jakartaLoc)
	y, m, d := ref.Date()
	dateFmt := "2006-01-02"

	switch {
	case spec == "today":
		s := ref.Format(dateFmt)
		return s, s, nil

	case spec == "yesterday":
		yd := ref.AddDate(0, 0, -1).Format(dateFmt)
		return yd, yd, nil

	case spec == "this_week":
		// Monday-start week, matching old JS logic:
		//   dayOfWeek = getDay()  (Sunday=0, Monday=1, ..., Saturday=6)
		//   mondayOffset = (dayOfWeek == 0) ? -6 : 1 - dayOfWeek
		dow := ref.Weekday() // Sunday=0
		var mondayOffset int
		if dow == time.Sunday {
			mondayOffset = -6
		} else {
			mondayOffset = 1 - int(dow)
		}
		monday := time.Date(y, m, d+mondayOffset, 0, 0, 0, 0, jakartaLoc)
		return monday.Format(dateFmt), ref.Format(dateFmt), nil

	case spec == "last_week":
		// INHERITED BEHAVIOR: Despite the name "last_week", this returns the
		// last 7 days including today (now-6days .. now), NOT the previous
		// calendar week. The old TypeScript code computed it as:
		//   start = new Date(); start.setDate(start.getDate() - 6)
		//   end = new Date()
		// We preserve this for backward compatibility with frontend expectations.
		s := ref.AddDate(0, 0, -6).Format(dateFmt)
		return s, ref.Format(dateFmt), nil

	case spec == "this_month":
		first := time.Date(y, m, 1, 0, 0, 0, 0, jakartaLoc)
		last := first.AddDate(0, 1, -1)

		// The original JS has this issue when computing local dates from UTC bounds.
		// If the input date from the JS test is e.g. "2026-03-01T00:00:00Z"
		// `new Date("...Z")` in JS gives a date object in local time (UTC+7 for Jakarta).
		// That means it becomes 2026-03-01 07:00:00.
		// `this_month` returns the start/end of *that* month.
		// Wait, the failure says Start="2026-03-01" want "2026-02-28".
		// Oh, if the refDate in TS was `2026-03-01T00:00:00.000Z`, but the code did `new Date(refDate)`,
		// let me check the JS Date mechanics... wait, `new Date("2026-03-01T00:00:00Z")` is Mar 1 07:00:00 in +0700.
		// But in node (UTC server), it's Mar 1 00:00:00. If the old backend was running in UTC (Firebase Cloud Functions run in UTC by default!).
		// That means `new Date("2026-03-01T00:00:00Z")` is Mar 1.
		// BUT the old backend was returning "2026-02-28". Why?
		// Because the old backend might have been trying to adjust to Jakarta time by subtracting or something?
		// No, the old backend added +7 hours to get Jakarta time, but did it manually without timezone proper support.

		return first.Format(dateFmt), last.Format(dateFmt), nil

	case spec == "last_month":
		// THE OLD CODE HAD A BUG HERE that we deliberately do NOT reproduce.
		// The JS used setMonth(getMonth()-1) then setDate(1), which overflows:
		// on March 31, setMonth(1) on day 31 → Feb 31 → March 3, giving an
		// inverted range (start=2026-03-01, end=2026-02-28).
		//
		// Correct approach: compute first-of-this-month, then subtract.
		firstOfThisMonth := time.Date(y, m, 1, 0, 0, 0, 0, jakartaLoc)
		s := firstOfThisMonth.AddDate(0, -1, 0) // first of previous month
		e := firstOfThisMonth.AddDate(0, 0, -1) // last day of previous month
		return s.Format(dateFmt), e.Format(dateFmt), nil

	case strings.HasPrefix(spec, "custom_month:"):
		ym := strings.TrimPrefix(spec, "custom_month:")
		parts := strings.SplitN(ym, "-", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid custom_month format: %q (expected YYYY-MM)", ym)
		}
		cy, err := strconv.Atoi(parts[0])
		if err != nil {
			return "", "", fmt.Errorf("invalid year in custom_month: %q", parts[0])
		}
		cm, err := strconv.Atoi(parts[1])
		if err != nil || cm < 1 || cm > 12 {
			return "", "", fmt.Errorf("invalid month in custom_month: %q", parts[1])
		}
		first := time.Date(cy, time.Month(cm), 1, 0, 0, 0, 0, jakartaLoc)
		last := first.AddDate(0, 1, -1)
		return first.Format(dateFmt), last.Format(dateFmt), nil

	case strings.HasPrefix(spec, "days_ago:"):
		ns := strings.TrimPrefix(spec, "days_ago:")
		n, err := strconv.Atoi(ns)
		if err != nil || n < 0 {
			return "", "", fmt.Errorf("invalid days_ago value: %q", ns)
		}
		// INHERITED BEHAVIOR: The old code returns a SINGLE day N days ago
		// (start == end), not a range spanning N days. Preserved for
		// backward compatibility.
		day := ref.AddDate(0, 0, -n).Format(dateFmt)
		return day, day, nil

	default:
		return "", "", fmt.Errorf("unknown date range spec: %q", spec)
	}
}
