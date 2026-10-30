package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestRequestRateLimiterUsesRemoteAddress(t *testing.T) {
	limiter := newRequestRateLimiter(2, time.Minute)
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for attempt := 1; attempt <= 3; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		request.RemoteAddr = "192.0.2.10:4567"
		request.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(attempt))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if attempt < 3 && response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200", attempt, response.Code)
		}
		if attempt == 3 {
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("third attempt status = %d, want 429", response.Code)
			}
			if response.Header().Get("Retry-After") == "" {
				t.Fatal("rate-limited response has no Retry-After header")
			}
		}
	}
}

func TestSecureHeadersMiddleware(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	handler := secureHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	for header, expected := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	} {
		if got := response.Header().Get(header); got != expected {
			t.Errorf("%s = %q, want %q", header, got, expected)
		}
	}
}
