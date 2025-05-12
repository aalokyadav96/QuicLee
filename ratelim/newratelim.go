package ratelim

import (
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/time/rate"
)

// RateLimiter structure
type RateLimiter struct {
	visitors map[string]*rate.Limiter
	mu       sync.Mutex
}

// Create a new rate limiter with 5 requests per minute
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*rate.Limiter),
	}
}

// Get or create a rate limiter for an IP
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, exists := rl.visitors[ip]; exists {
		return limiter
	}

	// Allow 5 requests per minute
	limiter := rate.NewLimiter(5, 1) // 5 requests per minute, 1 burst
	rl.visitors[ip] = limiter

	// Clean up old IPs after 10 minutes
	go func() {
		time.Sleep(10 * time.Minute)
		rl.mu.Lock()
		delete(rl.visitors, ip)
		rl.mu.Unlock()
	}()

	return limiter
}

// // Middleware to enforce rate limiting
// func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		ip := r.RemoteAddr
// 		limiter := rl.getLimiter(ip)

// 		if !limiter.Allow() {
// 			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
// 			return
// 		}

// 		next.ServeHTTP(w, r)
// 	})
// }

// Middleware to enforce rate limiting

func (rl *RateLimiter) Limit(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ip := r.RemoteAddr // Get the user's IP address
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		next(w, r, ps) // Call the next handler
	}
}
