package httputil

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter for API mutation endpoints.
type RateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	max      float64
	rate     float64 // tokens per second
	lastTick time.Time
}

// NewRateLimiter creates a rate limiter that allows max requests, refilling
// at rate requests per second.
func NewRateLimiter(max float64, perMinute float64) *RateLimiter {
	return &RateLimiter{
		tokens:   max,
		max:      max,
		rate:     perMinute / 60.0,
		lastTick: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTick).Seconds()
	rl.lastTick = now

	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.max {
		rl.tokens = rl.max
	}

	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

// RetryAfter returns the number of seconds until the next token is available.
func (rl *RateLimiter) RetryAfter() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.tokens >= 1 {
		return 0
	}
	wait := (1 - rl.tokens) / rl.rate
	return int(wait) + 1
}

// RateLimitMiddleware returns chi middleware that rate-limits mutation requests
// (non-GET, non-HEAD, non-OPTIONS). Read requests pass through freely.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			if !limiter.Allow() {
				retryAfter := limiter.RetryAfter()
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				WriteJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": "rate limit exceeded",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
