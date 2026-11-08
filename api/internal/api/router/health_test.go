package router

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/irgordon/kari/api/internal/api/handlers"
	authMiddleware "github.com/irgordon/kari/api/internal/api/middleware"
)

func TestLiveness(t *testing.T) {
	router := chi.NewRouter()
	registerHealthRoutes(router, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"status":"alive"}` {
		t.Fatalf("liveness response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestOriginRejectionSurvivesRateLimit(t *testing.T) {
	router := NewRouter(RouterConfig{AuthHandler: &handlers.AuthHandler{}, AuthMiddleware: &authMiddleware.AuthMiddleware{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	for i := 0; i < 60; i++ {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://api.example/api/v1/auth/unknown", nil))
	}
	for _, test := range []struct {
		method, path, origin string
		want                 int
	}{
		{http.MethodGet, "/api/v1/auth/unknown", "", http.StatusTooManyRequests},
		{http.MethodPost, "/api/v1/auth/logout", "http://attacker.example", http.StatusForbidden},
		{http.MethodPost, "/api/v1/auth/logout", "http://api.example", http.StatusTooManyRequests},
		{http.MethodGet, "/health", "", http.StatusOK},
	} {
		request := httptest.NewRequest(test.method, "http://api.example"+test.path, nil)
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s %s origin %q = %d, want %d", test.method, test.path, test.origin, response.Code, test.want)
		}
	}
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name   string
		check  func() error
		status int
	}{
		{"ready", func() error { return nil }, http.StatusOK},
		{"not ready", func() error { return errors.New("dependency unavailable") }, http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := chi.NewRouter()
			registerHealthRoutes(router, func(_ context.Context) error { return test.check() })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if recorder.Code != test.status {
				t.Fatalf("readiness status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}
