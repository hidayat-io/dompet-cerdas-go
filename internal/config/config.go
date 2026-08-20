package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// FirebaseProjectID is the GCP project ID for Firebase services.
	FirebaseProjectID string

	// GoogleCredentials is the file path to the Firebase service account JSON key.
	GoogleCredentials string

	// FirebaseStorageBucket is the GCS bucket for receipt images.
	// Optional — defaults to "{project-id}.appspot.com" if empty.
	FirebaseStorageBucket string

	// TelegramBotToken is the bot token from @BotFather.
	TelegramBotToken string

	// TelegramWebhookSecret is used to verify inbound Telegram webhook requests.
	TelegramWebhookSecret string

	// GeminiAPIKey is the API key for Google Gemini.
	GeminiAPIKey string

	// AntigravityBaseURL is the OpenAI-compatible base URL for Antigravity proxy.
	// Default: "http://127.0.0.1:3000/v1" when ANTIGRAVITY_BASE_URL is empty.
	AntigravityBaseURL string

	// AntigravityAPIKey is the proxy API key for Antigravity.
	AntigravityAPIKey string

	// AntigravityModel is the model ID for Antigravity (e.g. gemini-3.6-flash-high).
	AntigravityModel string

	// Port is the HTTP listen port. Default: "8080".
	Port string

	// Env is the runtime environment (development|staging|production). Default: "development".
	Env string

	// CORSAllowedOrigins is the list of origins permitted for CORS. Default: ["http://localhost:3000"].
	CORSAllowedOrigins []string

	// LogLevel controls slog verbering. One of: debug, info, warn, error. Default: "info".
	LogLevel string

	// TZ is the IANA timezone for date logic and cron. Default: "Asia/Jakarta".
	TZ string
}

// IsProduction returns true when running in the production environment.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// Load reads environment variables (optionally from .env) and returns a Config.
// It calls godotenv.Load() but does NOT fail if the .env file is absent —
// production deployments use real environment variables.
func Load() (*Config, error) {
	// Non-fatal: ignore error when .env is missing.
	_ = godotenv.Load()

	cfg := &Config{
		FirebaseProjectID:     os.Getenv("FIREBASE_PROJECT_ID"),
		GoogleCredentials:     os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		FirebaseStorageBucket: os.Getenv("FIREBASE_STORAGE_BUCKET"),
		TelegramBotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		GeminiAPIKey:          os.Getenv("GEMINI_API_KEY"),
		AntigravityBaseURL:    getEnvOr("ANTIGRAVITY_BASE_URL", "http://127.0.0.1:3000/v1"),
		AntigravityAPIKey:     os.Getenv("ANTIGRAVITY_API_KEY"),
		AntigravityModel:      getEnvOr("ANTIGRAVITY_MODEL", "gemini-3.6-flash-high"),
		Port:                  getEnvOr("PORT", "8080"),
		Env:                   getEnvOr("ENV", "development"),
		CORSAllowedOrigins:    parseCSV(getEnvOr("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		LogLevel:              getEnvOr("LOG_LEVEL", "info"),
		TZ:                    getEnvOr("TZ", "Asia/Jakarta"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.FirebaseStorageBucket == "" {
		cfg.FirebaseStorageBucket = cfg.FirebaseProjectID + ".appspot.com"
	}

	return cfg, nil
}

// Validate checks that all required configuration values are present.
// It returns an error listing ALL missing variables at once, not just the first,
// and sorts them so the message is stable across runs (Go map iteration order is
// randomised).
func (c *Config) Validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"FIREBASE_PROJECT_ID", c.FirebaseProjectID},
		{"GOOGLE_APPLICATION_CREDENTIALS", c.GoogleCredentials},
		{"TELEGRAM_BOT_TOKEN", c.TelegramBotToken},
	}

	var missing []string
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			missing = append(missing, r.name)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if strings.TrimSpace(c.GeminiAPIKey) == "" && strings.TrimSpace(c.AntigravityAPIKey) == "" {
		return fmt.Errorf("missing required environment variables: GEMINI_API_KEY or ANTIGRAVITY_API_KEY")
	}

	return nil
}

// getEnvOr returns the environment variable value or the default if empty/unset.
func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseCSV splits a comma-separated string into trimmed, non-empty parts.
func parseCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
