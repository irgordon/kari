package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func rateRequest(handler http.Handler, peer, forwarded string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/example", nil)
	request.RemoteAddr = peer
	request.Header.Set("X-Real-IP", forwarded)
	request.Header.Set("X-Forwarded-For", forwarded)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRateLimitClientIsolation(t *testing.T) {
	handler := RateLimitMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	var response *httptest.ResponseRecorder
	for i := 0; i < 60; i++ {
		response = rateRequest(handler, fmt.Sprintf("192.0.2.1:%d", 1000+i), fmt.Sprintf("203.0.113.%d", i+1))
	}
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("port/header changes bypassed limit: %d", response.Code)
	}
	if got := rateRequest(handler, "192.0.2.2:1000", "").Code; got != http.StatusOK {
		t.Fatalf("another client shared the exhausted budget: %d", got)
	}
}

func TestRateLimitTrustedProxy(t *testing.T) {
	handler := RateLimitMiddleware([]netip.Prefix{netip.MustParsePrefix("192.0.2.3/32")})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for i := 0; i < 60; i++ {
		rateRequest(handler, "192.0.2.3:1000", "203.0.113.1")
	}
	if got := rateRequest(handler, "192.0.2.3:2000", "203.0.113.1").Code; got != http.StatusTooManyRequests {
		t.Fatalf("same forwarded client bypassed limit: %d", got)
	}
	if got := rateRequest(handler, "192.0.2.3:2000", "203.0.113.2").Code; got != http.StatusOK {
		t.Fatalf("proxy clients shared a budget: %d", got)
	}
	if got := rateRequest(handler, "192.0.2.3:2000", "invalid").Code; got != http.StatusBadRequest {
		t.Fatalf("malformed trusted identity accepted: %d", got)
	}
}

func TestRateLimitStateIsBounded(t *testing.T) {
	handler := RateLimitMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	address := netip.MustParseAddr("2001:db8::1")
	for i := 0; i < 4096; i++ {
		if got := rateRequest(handler, netip.AddrPortFrom(address, 1000).String(), "").Code; got != http.StatusOK {
			t.Fatalf("client %d rejected before capacity: %d", i, got)
		}
		address = address.Next()
	}
	if got := rateRequest(handler, netip.AddrPortFrom(address, 1000).String(), "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("state capacity not enforced: %d", got)
	}
	if got := rateRequest(handler, "[2001:db8::1]:2000", "").Code; got != http.StatusOK {
		t.Fatalf("capacity evicted an existing client: %d", got)
	}
}
