package registration

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Options holds registration-related configuration
type Options struct {
	Enabled            bool    `long:"registration-enabled" env:"REGISTRATION_ENABLED" ini-name:"registration-enabled" description:"Enable user registration endpoint (disabled by default)"`
	RateLimitPerMinute float64 `long:"registration-rate-limit" env:"REGISTRATION_RATE_LIMIT" ini-name:"registration-rate-limit" description:"Maximum registration attempts per IP per minute" default:"5"`
	RateLimitBurst     int     `long:"registration-rate-burst" env:"REGISTRATION_RATE_BURST" ini-name:"registration-rate-burst" description:"Maximum burst of registration attempts per IP" default:"10"`
}

// NewOptions creates a new Options with defaults
func NewOptions() *Options {
	return &Options{
		Enabled:            false, // Disabled by default
		RateLimitPerMinute: 5,
		RateLimitBurst:     10,
	}
}

// RateLimiter manages per-IP rate limiting for registration
type RateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int

	// cleanup old entries periodically
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given options
func NewRateLimiter(opts *Options) *RateLimiter {
	rl := &RateLimiter{
		limiters:        make(map[string]*rate.Limiter),
		rate:            rate.Limit(opts.RateLimitPerMinute / 60), // convert per-minute to per-second
		burst:           opts.RateLimitBurst,
		cleanupInterval: 10 * time.Minute,
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given IP is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = limiter
	}
	rl.mu.Unlock()

	return limiter.Allow()
}

// cleanup removes old limiters periodically to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			// Clear all limiters - they will be recreated on next request
			// This is simpler than tracking last access time
			rl.limiters = make(map[string]*rate.Limiter)
			rl.mu.Unlock()
		case <-rl.stopCleanup:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (rl *RateLimiter) Close() {
	close(rl.stopCleanup)
}
