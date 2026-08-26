package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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
