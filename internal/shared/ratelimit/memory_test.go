package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_AllowsUpToMax(t *testing.T) {
	ml := NewMemoryLimiter()
	defer ml.Stop()

	for i := 0; i < maxRequests; i++ {
		allowed, retryAfter := ml.Allow("user-1")
		if !allowed {
			t.Fatalf("request %d denied, want allowed", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("request %d retryAfter = %v, want 0", i+1, retryAfter)
		}
	}

	allowed, retryAfter := ml.Allow("user-1")
	if allowed {
		t.Errorf("request %d allowed, want denied", maxRequests+1)
	}
	if retryAfter <= 0 || retryAfter > rateWindow {
		t.Errorf("retryAfter = %v, want between 0 and %v", retryAfter, rateWindow)
	}
}

func TestMemoryLimiter_KeysAreIndependent(t *testing.T) {
	ml := NewMemoryLimiter()
	defer ml.Stop()

	for i := 0; i < maxRequests; i++ {
		ml.Allow("user-1")
	}

	if allowed, _ := ml.Allow("user-1"); allowed {
		t.Error("user-1 should be throttled")
	}
	if allowed, _ := ml.Allow("user-2"); !allowed {
		t.Error("user-2 should not be affected by user-1's usage")
	}
}

func TestMemoryLimiter_IsDuplicate(t *testing.T) {
	ml := NewMemoryLimiter()
	defer ml.Stop()

	if ml.IsDuplicate("user-1", "makan siang 25rb") {
		t.Error("first occurrence reported as duplicate")
	}
	if !ml.IsDuplicate("user-1", "makan siang 25rb") {
		t.Error("second identical payload not reported as duplicate")
	}
	if ml.IsDuplicate("user-1", "parkir 5rb") {
		t.Error("different payload reported as duplicate")
	}
	if ml.IsDuplicate("user-2", "makan siang 25rb") {
		t.Error("same payload from a different key reported as duplicate")
	}
}

// The janitor must not leave a key throttled forever: once its window has
// elapsed, evict should drop the entry entirely.
func TestMemoryLimiter_EvictDropsStaleEntries(t *testing.T) {
	ml := NewMemoryLimiter()
	defer ml.Stop()

	for i := 0; i < maxRequests; i++ {
		ml.Allow("user-1")
	}
	ml.IsDuplicate("user-1", "payload")

	ml.mu.Lock()
	stale := time.Now().Add(-2 * rateWindow)
	ml.entries["user-1"].timestamps = []time.Time{stale}
	ml.dedupes["user-1|payload"].expireAt = time.Now().Add(-time.Second)
	ml.mu.Unlock()

	ml.evict()

	ml.mu.Lock()
	entryCount := len(ml.entries)
	dedupeCount := len(ml.dedupes)
	ml.mu.Unlock()

	if entryCount != 0 {
		t.Errorf("entries after evict = %d, want 0", entryCount)
	}
	if dedupeCount != 0 {
		t.Errorf("dedupes after evict = %d, want 0", dedupeCount)
	}

	if allowed, _ := ml.Allow("user-1"); !allowed {
		t.Error("user-1 still throttled after its window elapsed")
	}
}

func TestMemoryLimiter_ConcurrentAccess(t *testing.T) {
	ml := NewMemoryLimiter()
	defer ml.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ml.Allow("shared-key")
			ml.IsDuplicate("shared-key", "payload")
		}(i)
	}
	wg.Wait()
}

// MemoryLimiter must satisfy Limiter so a Redis implementation can replace it
// without touching call sites.
func TestMemoryLimiterSatisfiesInterface(t *testing.T) {
	var _ Limiter = NewMemoryLimiter()
}
