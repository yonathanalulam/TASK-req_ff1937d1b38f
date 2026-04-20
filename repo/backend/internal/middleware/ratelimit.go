package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/apierr"
	"github.com/eagle-point/service-portal/internal/auth"
)

// RateLimiter implements an in-memory sliding-window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
	limit   int
	window  time.Duration
}

// NewRateLimiter creates a limiter with the given max requests per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Limit returns a Gin middleware that enforces the rate limit per authenticated user.
// For unauthenticated routes, the limit is applied per client IP.
func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := rl.key(c)

		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		// Trim old entries
		old := rl.buckets[key]
		fresh := old[:0]
		for _, t := range old {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}

		if len(fresh) >= rl.limit {
			rl.mu.Unlock()
			retryAfter := int(rl.window.Seconds())
			apierr.TooManyRequests(c, retryAfter)
			return
		}

		rl.buckets[key] = append(fresh, now)
		rl.mu.Unlock()

		c.Next()
	}
}

func (rl *RateLimiter) key(c *gin.Context) string {
	if id := c.GetUint64(auth.CtxUserID); id != 0 {
		return "u:" + uint64ToStr(id)
	}
	return "ip:" + c.ClientIP()
}

func uint64ToStr(n uint64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// ─── Pre-built limiters ──────────────────────────────────────────────────────

// NewGeneralLimiter returns the 60 req/min per user limiter.
func NewGeneralLimiter() *RateLimiter {
	return NewRateLimiter(60, time.Minute)
}

// NewReviewReportLimiter returns the 10 submissions/hour per user limiter.
func NewReviewReportLimiter() *RateLimiter {
	return NewRateLimiter(10, time.Hour)
}
