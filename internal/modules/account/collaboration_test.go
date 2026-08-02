package account

import "testing"

func TestNormalizeInviteCode(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  ab-cd 12 ", "ABCD12"},
		{"xyz", "XYZ"},
		{"", ""},
		{"!!!", ""},
	}
	for _, tt := range tests {
		if got := normalizeInviteCode(tt.in); got != tt.want {
			t.Errorf("normalizeInviteCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildMember(t *testing.T) {
	m := buildMember("u1", "OWNER", "a@b.c", "Name", "2026-01-01T00:00:00Z")
	if m.UserID != "u1" || m.Role != "OWNER" || m.Email != "a@b.c" {
		t.Fatalf("unexpected member: %+v", m)
	}
}
