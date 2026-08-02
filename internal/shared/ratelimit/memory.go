package ratelimit

import (
	"sync"
	"time"
)

// Production constants ported from the old Node.js backend.
const (
	// maxRequests is the number of requests allowed per rate window.
	// Inherited: 5 requests per 10 seconds per key.
	maxRequests = 5

	// rateWindow is the sliding window duration for rate limiting.
	rateWindow = 10 * time.Second

	// dedupeWindow is the duration within which identical key+payload
	// combinations are treated as duplicates.
	// Inherited: 5-second duplicate-payload suppression.
	dedupeWindow = 5 * time.Second

	// janitorInterval controls how often stale entries are evicted.
	janitorInterval = 30 * time.Second
)

// entry tracks timestamps for a single rate-limit key.
type entry struct {
	timestamps []time.Time
}

// dedupeEntry tracks a payload hash and its expiry for duplicate detection.
type dedupeEntry struct {
	payload  string
	expireAt time.Time
}

// MemoryLimiter is an in-memory Limiter backed by a mutex-guarded map.
// It includes a background janitor goroutine that evicts stale entries
// to prevent unbounded memory growth (the old Node.js version leaked).
type MemoryLimiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	dedupes map[string]*dedupeEntry
	stop    chan struct{}
}

// NewMemoryLimiter creates a new in-memory rate limiter and starts its
// background janitor goroutine. Call Stop() when done to release resources.
func NewMemoryLimiter() *MemoryLimiter {
	ml := &MemoryLimiter{
		entries: make(map[string]*entry),
		dedupes: make(map[string]*dedupeEntry),
		stop:    make(chan struct{}),
	}
	go ml.janitor()
	return ml
}

// Allow checks whether the key is within its rate limit.
// Thread-safe.
func (ml *MemoryLimiter) Allow(key string) (bool, time.Duration) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateWindow)

	e, ok := ml.entries[key]
	if !ok {
		e = &entry{}
		ml.entries[key] = e
	}

	// Prune timestamps outside the window.
	valid := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	e.timestamps = valid

	if len(e.timestamps) >= maxRequests {
		// Calculate retry-after from the oldest timestamp in the window.
		oldest := e.timestamps[0]
		retryAfter := oldest.Add(rateWindow).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	e.timestamps = append(e.timestamps, now)
	return true, 0
}

// IsDuplicate returns true if the same key+payload was seen within the
// deduplication window. If not a duplicate, records it for future checks.
// Thread-safe.
func (ml *MemoryLimiter) IsDuplicate(key, payload string) bool {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	now := time.Now()
	dedupeKey := key + "|" + payload

	if de, ok := ml.dedupes[dedupeKey]; ok {
		if now.Before(de.expireAt) {
			return true
		}
	}

	ml.dedupes[dedupeKey] = &dedupeEntry{
		payload:  payload,
		expireAt: now.Add(dedupeWindow),
	}
	return false
}

// Stop terminates the background janitor goroutine.
func (ml *MemoryLimiter) Stop() {
	close(ml.stop)
}

// janitor periodically evicts stale rate-limit and dedupe entries.
func (ml *MemoryLimiter) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ml.stop:
			return
		case <-ticker.C:
			ml.evict()
		}
	}
}

// evict removes expired entries from both maps.
func (ml *MemoryLimiter) evict() {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateWindow)

	// Evict rate-limit entries with no recent timestamps.
	for key, e := range ml.entries {
		valid := e.timestamps[:0]
		for _, ts := range e.timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		if len(valid) == 0 {
			delete(ml.entries, key)
		} else {
			e.timestamps = valid
		}
	}

	// Evict expired dedupe entries.
	for key, de := range ml.dedupes {
		if now.After(de.expireAt) {
			delete(ml.dedupes, key)
		}
	}
}
