package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// RateLimiter implements token-bucket rate limiting with Redis.
type RateLimiter struct {
	redis     *redis.Client
	maxReqs   int
	windowSec int
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rdb *redis.Client, maxReqs, windowSec int) *RateLimiter {
	return &RateLimiter{
		redis:     rdb,
		maxReqs:   maxReqs,
		windowSec: windowSec,
	}
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(ctx context.Context, key string) (bool, int, error) {
	redisKey := fmt.Sprintf("ratelimit:%s", key)

	count, err := rl.redis.Incr(ctx, redisKey).Result()
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("Rate limiter Redis error")
		return true, 0, err // fail open
	}

	if count == 1 {
		rl.redis.Expire(ctx, redisKey, time.Duration(rl.windowSec)*time.Second)
	}

	remaining := rl.maxReqs - int(count)
	if remaining < 0 {
		remaining = 0
	}

	return int(count) <= rl.maxReqs, remaining, nil
}

// Middleware returns HTTP middleware for rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		allowed, remaining, _ := rl.Allow(r.Context(), ip)

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.maxReqs))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !allowed {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders adds security headers to responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// CORS adds CORS headers for the dashboard.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
