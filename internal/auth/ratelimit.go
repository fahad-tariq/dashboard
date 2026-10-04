package auth

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

const (
	maxAttempts = 5
	window      = time.Minute

	// defaultTracked caps how many IPs or accounts are remembered. When full,
	// the least recently seen entry is dropped; nothing is ever reset in bulk.
	defaultTracked = 4096

	// Account delay: free attempts, then 1s, 2s, 4s... capped, forgotten after
	// a quiet period. A delay, never a lockout, so an attacker cannot lock the
	// only user out.
	freeFailures    = 3
	maxAccountDelay = 8 * time.Second
	failureMemory   = 15 * time.Minute
)

// lru is a fixed-capacity map that evicts the least recently used key.
// Callers hold their own lock.
type lru[V any] struct {
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

type lruEntry[V any] struct {
	key   string
	value V
}

func newLRU[V any](capacity int) *lru[V] {
	return &lru[V]{capacity: capacity, order: list.New(), items: make(map[string]*list.Element)}
}

// get returns the entry for key, creating it with init if absent, and marks
// it most recently used.
func (c *lru[V]) get(key string, init func() V) *V {
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return &el.Value.(*lruEntry[V]).value
	}
	e := &lruEntry[V]{key: key, value: init()}
	c.items[key] = c.order.PushFront(e)
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry[V]).key)
	}
	return &e.value
}

func (c *lru[V]) peek(key string) (V, bool) {
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	return el.Value.(*lruEntry[V]).value, true
}

func (c *lru[V]) remove(key string) {
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

type attempt struct {
	count    int
	windowAt time.Time
}

// RateLimiter tracks login attempts per client IP.
type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	attempts *lru[attempt]
}

func NewRateLimiter() *RateLimiter {
	return NewRateLimiterWithCapacity(defaultTracked)
}

// NewRateLimiterWithCapacity bounds the number of IPs tracked at once.
func NewRateLimiterWithCapacity(capacity int) *RateLimiter {
	return NewRateLimiterWithLimit(maxAttempts, capacity)
}

// NewRateLimiterWithLimit allows limit attempts per IP per minute.
func NewRateLimiterWithLimit(limit, capacity int) *RateLimiter {
	return &RateLimiter{limit: limit, attempts: newLRU[attempt](capacity)}
}

// Allow returns true if the IP has not exceeded the rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	a := rl.attempts.get(ip, func() attempt { return attempt{windowAt: now.Add(window)} })
	if now.After(a.windowAt) {
		*a = attempt{windowAt: now.Add(window)}
	}
	a.count++
	return a.count <= rl.limit
}

// RetryAfter returns the duration until the rate limit resets for the given IP.
func (rl *RateLimiter) RetryAfter(ip string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, ok := rl.attempts.peek(ip)
	if !ok {
		return 0
	}
	return max(time.Until(a.windowAt), 0)
}

// Len reports how many IPs are currently tracked.
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.attempts.order.Len()
}

type accountFailures struct {
	count int
	last  time.Time
}

// AccountDelay slows repeated failed logins for one account, whichever IPs
// they come from. Real accounts and unknown emails are tracked separately so
// a flood of made-up emails cannot evict a real account's history; both use
// the same schedule, so the delay does not reveal which accounts exist.
type AccountDelay struct {
	mu      sync.Mutex
	known   *lru[accountFailures]
	unknown *lru[accountFailures]
}

// knownTracked bounds real accounts; this is a single-owner deployment.
const knownTracked = 64

func NewAccountDelay() *AccountDelay {
	return &AccountDelay{
		known:   newLRU[accountFailures](knownTracked),
		unknown: newLRU[accountFailures](defaultTracked),
	}
}

func accountKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Delay returns how long to wait before checking a password for email.
func (d *AccountDelay) Delay(email string) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := accountKey(email)
	f, ok := d.known.peek(key)
	if !ok {
		f, ok = d.unknown.peek(key)
	}
	if !ok || time.Since(f.last) > failureMemory || f.count < freeFailures {
		return 0
	}
	shift := min(f.count-freeFailures, 8)
	return min(time.Second<<shift, maxAccountDelay)
}

// Fail records a failed login for email; known says whether the account exists.
func (d *AccountDelay) Fail(email string, known bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bucket := d.unknown
	if known {
		bucket = d.known
	}
	now := time.Now()
	f := bucket.get(accountKey(email), func() accountFailures { return accountFailures{} })
	if now.Sub(f.last) > failureMemory {
		f.count = 0
	}
	f.count++
	f.last = now
}

// Succeed clears the failure history for email.
func (d *AccountDelay) Succeed(email string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.known.remove(accountKey(email))
	d.unknown.remove(accountKey(email))
}
