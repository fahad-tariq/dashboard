package auth

import (
	"sync"
	"time"
)

const (
	maxAttempts = 5
	window      = time.Minute
)

type attempt struct {
	count    int
	windowAt time.Time
}

// RateLimiter tracks login attempts per IP address.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{attempts: make(map[string]*attempt)}
	go rl.cleanup()
	return rl
}

// Allow returns true if the IP has not exceeded the rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	a, ok := rl.attempts[ip]
	if !ok || now.After(a.windowAt) {
		rl.attempts[ip] = &attempt{count: 1, windowAt: now.Add(window)}
		return true
	}
	a.count++
	return a.count <= maxAttempts
}

// RetryAfter returns the duration until the rate limit resets for the given IP.
func (rl *RateLimiter) RetryAfter(ip string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, ok := rl.attempts[ip]
	if !ok {
		return 0
	}
	d := time.Until(a.windowAt)
	if d < 0 {
		return 0
	}
	return d
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		rl.mu.Lock()
		now := time.Now()
		for ip, a := range rl.attempts {
			if now.After(a.windowAt) {
				delete(rl.attempts, ip)
			}
		}
		rl.mu.Unlock()
	}
}
