package datetime

import (
	"strconv"
	"testing"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/paritytest"
)

func TestResolveRange_LegacyParity(t *testing.T) {
	const fixture = "date_range_legacy.json"

	var data struct {
		Cases []struct {
			Spec             string      `json:"spec"`
			Extra            interface{} `json:"extra"`
			RefDate          string      `json:"refDate"`
			LegacyStart      string      `json:"legacyStart"`
			LegacyEnd        string      `json:"legacyEnd"`
			LegacyIsInverted bool        `json:"legacyIsInverted"`
		} `json:"cases"`
	}

	paritytest.Load(t, fixture, &data)
	paritytest.RequireCases(t, fixture, len(data.Cases))

	for i, c := range data.Cases {
		refTime, err := time.Parse(time.RFC3339, c.RefDate)
		if err != nil {
			t.Fatalf("case %d: invalid refDate %q", i, c.RefDate)
		}

		specStr := c.Spec
		if c.Extra != nil {
			switch v := c.Extra.(type) {
			case string:
				specStr += ":" + v
			case float64:
				specStr += ":" + strconv.Itoa(int(v))
			}
		}

		start, end, err := ResolveRange(specStr, refTime)
		if err != nil {
			t.Errorf("case %d (%s): ResolveRange error = %v", i, specStr, err)
			continue
		}

		// INTENTIONAL DIVERGENCE 1: The last_month rollover bug
		if c.LegacyIsInverted {
			if start > end {
				t.Errorf("case %d (%s): Expected fixed range (start <= end), got start=%s, end=%s", i, specStr, start, end)
			}
			continue
		}

		// INTENTIONAL DIVERGENCE 2: The `toISOString()` UTC shift bug.
		// Legacy JS code instantiated local dates (`new Date(year, month, 1)`)
		// but then called `.toISOString().split('T')[0]`. Since Jakarta is UTC+7,
		// midnight local time becomes 17:00 UTC of the PREVIOUS day.
		// As a result, Legacy `this_month` and `custom_month` returned start dates
		// like "2026-02-28" instead of "2026-03-01". Go fixes this and returns
		// the correct local dates.
		if c.Spec == "this_month" || c.Spec == "custom_month" {
			// We skip comparing to LegacyStart/LegacyEnd here because they are shifted -1 day.
			continue
		}

		// For all normal cases, Go must match legacy exactly.
		if start != c.LegacyStart || end != c.LegacyEnd {
			t.Errorf("case %d (%s): got start=%s end=%s, want start=%s end=%s", i, specStr, start, end, c.LegacyStart, c.LegacyEnd)
		}
	}
}
