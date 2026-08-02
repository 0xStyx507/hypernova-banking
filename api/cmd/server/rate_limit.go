package main

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultRequestsPerMinute = 120

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

// rateLimiter provides a bounded local guard for a single API instance. A
// distributed deployment should replace it with a shared store at the edge.
type rateLimiter struct {
	mu          sync.Mutex
	entries     map[string]rateLimitEntry
	limit       int
	window      time.Duration
	lastCleanup time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	if limit <= 0 {
		limit = defaultRequestsPerMinute
	}
	return &rateLimiter{entries: make(map[string]rateLimitEntry), limit: limit, window: time.Minute}
}

func (limiter *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientAddress(r)) {
			w.Header().Set("Retry-After", "60")
			writeErrorCode(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (limiter *rateLimiter) allow(key string) bool {
	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if now.Sub(limiter.lastCleanup) >= limiter.window {
		for entryKey, entry := range limiter.entries {
			if now.Sub(entry.windowStart) >= limiter.window {
				delete(limiter.entries, entryKey)
			}
		}
		limiter.lastCleanup = now
	}
	entry := limiter.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= limiter.window {
		limiter.entries[key] = rateLimitEntry{windowStart: now, count: 1}
		return true
	}
	if entry.count >= limiter.limit {
		return false
	}
	entry.count++
	limiter.entries[key] = entry
	return true
}

func clientAddress(r *http.Request) string {
	address := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	if address == "" {
		return "unknown"
	}
	return address
}

func rateLimitFromEnv() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("API_RATE_LIMIT_PER_MINUTE")))
	if err != nil || value <= 0 {
		return defaultRequestsPerMinute
	}
	return value
}
