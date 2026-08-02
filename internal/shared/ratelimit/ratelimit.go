// Package ratelimit defines the Limiter interface for rate limiting and
// duplicate-payload suppression. Implementations must be safe for concurrent use.
package ratelimit

import "time"

// Limiter controls request rate and detects duplicate payloads.
type Limiter interface {
	// Allow checks whether the given key is within its rate limit.
	// Returns allowed=true if the request should proceed, or allowed=false
	// with retryAfter indicating how long the caller should wait.
	Allow(key string) (allowed bool, retryAfter time.Duration)

	// IsDuplicate returns true if the same key+payload combination was seen
	// within the deduplication window. Used to suppress accidental double-taps
	// (e.g. Telegram webhook retries).
	IsDuplicate(key, payload string) bool
}
