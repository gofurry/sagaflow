package api

import (
	"testing"
	"time"
)

func TestLoginRateLimiterBlocksAndRecovers(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	for range 5 {
		if !limiter.Allow("127.0.0.1", "creator") {
			t.Fatal("limiter blocked before the configured failure threshold")
		}
		limiter.RecordFailure("127.0.0.1", "creator")
	}
	if limiter.Allow("127.0.0.1", "creator") {
		t.Fatal("expected the sixth attempt for the same account and IP to be blocked")
	}
	if !limiter.Allow("127.0.0.1", "another-user") {
		t.Fatal("a different account should remain available below the IP-wide threshold")
	}

	now = now.Add(time.Minute)
	if !limiter.Allow("127.0.0.1", "creator") {
		t.Fatal("expected the limiter to recover after its window")
	}
}

func TestLoginRateLimiterClearsAccountPairAfterSuccess(t *testing.T) {
	limiter := newLoginRateLimiter()
	for range 4 {
		limiter.RecordFailure("127.0.0.1", "creator")
	}
	limiter.RecordSuccess("127.0.0.1", "creator")
	if !limiter.Allow("127.0.0.1", "creator") {
		t.Fatal("successful login should clear the account and IP pair")
	}
}
