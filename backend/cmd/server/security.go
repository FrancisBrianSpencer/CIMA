package main

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type requestWindow struct {
	startedAt time.Time
	count     int
}

type requestRateLimiter struct {
	mu      sync.Mutex
	clients map[string]requestWindow
	limit   int
	window  time.Duration
	checks  uint64
}

// newRequestRateLimiter creates a bounded per-client fixed-window limiter for API routes.
func newRequestRateLimiter(limit int, window time.Duration) *requestRateLimiter {
	return &requestRateLimiter{
		clients: make(map[string]requestWindow),
		limit:   limit,
		window:  window,
	}
}

// Middleware rejects excessive requests using the remote socket address, not caller-supplied proxy headers.
func (limiter *requestRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		client := requestClientIP(req.RemoteAddr)
		now := time.Now()
		limiter.mu.Lock()
		limiter.checks++
		if limiter.checks%256 == 0 {
			for address, state := range limiter.clients {
				if now.Sub(state.startedAt) >= limiter.window {
					delete(limiter.clients, address)
				}
			}
		}
		state, exists := limiter.clients[client]
		if !exists || now.Sub(state.startedAt) >= limiter.window {
			if len(limiter.clients) >= 10000 && !exists {
				limiter.mu.Unlock()
				w.Header().Set("Retry-After", strconvSeconds(limiter.window))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Límite temporal de solicitudes alcanzado."})
				return
			}
			state = requestWindow{startedAt: now}
		}
		if state.count >= limiter.limit {
			remaining := limiter.window - now.Sub(state.startedAt)
			limiter.mu.Unlock()
			w.Header().Set("Retry-After", strconvSeconds(remaining))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Demasiadas solicitudes. Inténtalo nuevamente más tarde."})
			return
		}
		state.count++
		limiter.clients[client] = state
		limiter.mu.Unlock()
		next.ServeHTTP(w, req)
	})
}

// secureHeadersMiddleware adds browser security headers to API and redirect responses.
func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if strings.EqualFold(strings.TrimSpace(getenv("APP_ENV", "development")), "production") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, req)
	})
}

// requestBodyLimitMiddleware caps API bodies before JSON decoding or base64 expansion.
func requestBodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Body != nil {
				req.Body = http.MaxBytesReader(w, req.Body, maxBytes)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// requestClientIP extracts an IP address from RemoteAddr without trusting X-Forwarded-For.
func requestClientIP(remoteAddress string) string {
	if host, _, err := net.SplitHostPort(remoteAddress); err == nil {
		return host
	}
	return remoteAddress
}

// strconvSeconds formats a positive Retry-After value rounded up to a whole second.
func strconvSeconds(duration time.Duration) string {
	seconds := int((duration + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
