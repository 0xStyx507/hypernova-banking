package main

import "testing"

func TestRateLimiterRejectsAfterConfiguredLimit(t *testing.T) {
	limiter := newRateLimiter(2)
	if !limiter.allow("198.51.100.10") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.allow("198.51.100.10") {
		t.Fatal("second request should be allowed")
	}
	if limiter.allow("198.51.100.10") {
		t.Fatal("third request should be rejected")
	}
	if !limiter.allow("198.51.100.11") {
		t.Fatal("a different client should have an independent window")
	}
}
