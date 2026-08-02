package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	router := newRouter(nil, readyReadiness{}, nil)
	paths := []string{"/healthz", "/readyz", "/api/v1/health"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", res.Code)
			}
			if got := res.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("expected JSON content type, got %q", got)
			}

			var body healthResponse
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatalf("decode health response: %v", err)
			}
			if body != (healthResponse{Status: "ok", Service: serviceName}) {
				t.Fatalf("unexpected health response: %+v", body)
			}
		})
	}
}

func TestReadinessFailsWithoutDatabase(t *testing.T) {
	router := newRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", res.Code)
	}
}

func TestAPIPort(t *testing.T) {
	if got := apiPort(""); got != defaultAPIPort {
		t.Fatalf("expected default port %q, got %q", defaultAPIPort, got)
	}
	if got := apiPort("9090"); got != "9090" {
		t.Fatalf("expected configured port, got %q", got)
	}
}

func TestResolveLedgerAddress(t *testing.T) {
	if got, err := resolveLedgerAddress("127.0.0.1:3000"); err != nil || got != "127.0.0.1:3000" {
		t.Fatalf("expected numeric ledger address, got %q, %v", got, err)
	}
	if _, err := resolveLedgerAddress("not-an-address"); err == nil {
		t.Fatal("expected malformed ledger address to fail")
	}
}

type readyReadiness struct{}

func (readyReadiness) Ping(context.Context) error { return nil }
