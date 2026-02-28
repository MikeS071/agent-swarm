package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("missing token", func(t *testing.T) {
		h := BearerAuthMiddleware("secret")(next)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		h := BearerAuthMiddleware("secret")(next)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		req.Header.Set("Authorization", "Bearer nope")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		h := BearerAuthMiddleware("secret")(next)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		req.Header.Set("Authorization", "Bearer secret")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})

	t.Run("disabled auth", func(t *testing.T) {
		h := BearerAuthMiddleware("")(next)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
	})
}

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := CORSMiddleware([]string{"https://example.com"})(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	req.Header.Set("Origin", "https://example.com")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("unexpected allow origin %q", got)
	}
}
