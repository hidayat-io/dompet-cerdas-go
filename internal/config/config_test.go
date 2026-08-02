package config

import (
	"testing"
)

func TestValidate_ReportsAllMissingVars(t *testing.T) {
	cfg := &Config{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error when every required var is unset")
	}

	want := "missing required environment variables: FIREBASE_PROJECT_ID, GEMINI_API_KEY, GOOGLE_APPLICATION_CREDENTIALS, TELEGRAM_BOT_TOKEN"
	if err.Error() != want {
		t.Errorf("Validate() error = %q, want %q", err.Error(), want)
	}
}

func TestValidate_TreatsWhitespaceAsMissing(t *testing.T) {
	cfg := &Config{
		FirebaseProjectID: "   ",
		GoogleCredentials: "/path/sa.json",
		TelegramBotToken:  "token",
		GeminiAPIKey:      "key",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a whitespace-only value")
	}
	want := "missing required environment variables: FIREBASE_PROJECT_ID"
	if err.Error() != want {
		t.Errorf("Validate() error = %q, want %q", err.Error(), want)
	}
}

func TestValidate_PassesWhenComplete(t *testing.T) {
	cfg := &Config{
		FirebaseProjectID: "proj",
		GoogleCredentials: "/path/sa.json",
		TelegramBotToken:  "token",
		GeminiAPIKey:      "key",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{env: "production", want: true},
		{env: "development", want: false},
		{env: "staging", want: false},
		{env: "", want: false},
		{env: "Production", want: false},
	}

	for _, tt := range tests {
		cfg := &Config{Env: tt.env}
		if got := cfg.IsProduction(); got != tt.want {
			t.Errorf("IsProduction() with Env=%q = %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "single", input: "https://a.web.app", want: []string{"https://a.web.app"}},
		{
			name:  "multiple_with_spaces",
			input: "https://a.web.app, https://b.web.app",
			want:  []string{"https://a.web.app", "https://b.web.app"},
		},
		{name: "drops_empty_segments", input: "a,,b,", want: []string{"a", "b"}},
		{name: "empty_string", input: "", want: []string{}},
		{name: "only_separators", input: ",,,", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSV(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseCSV(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetEnvOr(t *testing.T) {
	t.Setenv("DOMPET_TEST_SET", "value")
	t.Setenv("DOMPET_TEST_EMPTY", "")

	if got := getEnvOr("DOMPET_TEST_SET", "fallback"); got != "value" {
		t.Errorf("getEnvOr with a set var = %q, want %q", got, "value")
	}
	if got := getEnvOr("DOMPET_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("getEnvOr with an empty var = %q, want %q", got, "fallback")
	}
	if got := getEnvOr("DOMPET_TEST_UNSET", "fallback"); got != "fallback" {
		t.Errorf("getEnvOr with an unset var = %q, want %q", got, "fallback")
	}
}
