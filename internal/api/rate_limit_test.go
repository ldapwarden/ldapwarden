package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestLoginRateLimit_Triggers429 confirms the rate-limit middleware actually
// rejects after the configured budget. We mount it on a stand-in handler
// rather than on the full server so the test is hermetic — the only thing
// being asserted is that loginRateLimit() yields 429 once you exceed the
// loginPerMinute constant.
func TestLoginRateLimit_Triggers429(t *testing.T) {
	r := chi.NewRouter()
	r.With(loginRateLimit()).Post("/login", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // mimic a failed login
	})

	for i := 1; i <= loginPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "10.0.0.42:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status=%d, want 401 (under budget)", i, rec.Code)
		}
	}

	// One more — should be throttled.
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.42:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("over-budget request: status=%d, want 429", rec.Code)
	}
}

// TestLoginRateLimit_PerIP confirms the limiter is keyed on remote IP — a
// different client should not be affected by another's exhausted budget.
func TestLoginRateLimit_PerIP(t *testing.T) {
	r := chi.NewRouter()
	r.With(loginRateLimit()).Post("/login", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	// Exhaust IP A's budget.
	for i := 0; i < loginPerMinute+1; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	// IP B is fresh.
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Errorf("IP B unexpectedly throttled by IP A's bucket")
	}
}
