package utils

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple per-IP rate limiter using a token bucket approach.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	interval time.Duration
}

type visitor struct {
	count    int
	lastSeen time.Time
}

var globalLimiter *RateLimiter

// InitRateLimiter initializes the global rate limiter.
// rate is the maximum number of requests per interval.
func InitRateLimiter(rate int, interval time.Duration) {
	globalLimiter = &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
	}
	// Start a background cleaner.
	go func() {
		for {
			time.Sleep(interval)
			globalLimiter.cleanup()
		}
	}()
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, v := range rl.visitors {
		if time.Since(v.lastSeen) > rl.interval {
			delete(rl.visitors, ip)
		}
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}
	if time.Since(v.lastSeen) > rl.interval {
		v.count = 1
		v.lastSeen = time.Now()
		return true
	}
	if v.count >= rl.rate {
		return false
	}
	v.count++
	return true
}

// RateLimitMiddleware limits the number of requests per IP address.
func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if globalLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		ip := r.RemoteAddr
		if !globalLimiter.allow(ip) {
			SendErrorResponse(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware sets CORS headers and handles preflight OPTIONS requests.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Token, X-Timestamp, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// XSSProtectionMiddleware sets security headers to help prevent XSS attacks.
func XSSProtectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}
