package domain

import (
	"encoding/json"
	"testing"
)

// A malformed score must degrade to zero (confirmation) rather than fail the
// whole receipt parse, because zero can never open the auto-save gate.
func TestConfidenceScore_TolerantUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		json string
		want ConfidenceScore
	}{
		{name: "integer", json: `{"confidenceScore":95}`, want: 95},
		{name: "float_truncated", json: `{"confidenceScore":92.7}`, want: 92},
		{name: "quoted_number", json: `{"confidenceScore":"88"}`, want: 88},
		{name: "quoted_garbage", json: `{"confidenceScore":"high"}`, want: 0},
		{name: "absent", json: `{}`, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data ReceiptData
			if err := json.Unmarshal([]byte(tt.json), &data); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if data.ConfidenceScore != tt.want {
				t.Errorf("ConfidenceScore = %d, want %d", data.ConfidenceScore, tt.want)
			}
		})
	}
}
