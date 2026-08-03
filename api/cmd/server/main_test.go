package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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

func TestRequestIDMiddlewarePreservesValidID(t *testing.T) {
	expectedID := "11111111-1111-4111-8111-111111111111"
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestID(r); got != expectedID {
			t.Errorf("expected request id in context %q, got %q", expectedID, got)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", expectedID)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if got := res.Header().Get("X-Request-ID"); got != expectedID {
		t.Fatalf("expected response request id %q, got %q", expectedID, got)
	}
}

func TestRequestIDMiddlewareReplacesInvalidID(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := uuid.Parse(requestID(r)); err != nil {
			t.Errorf("expected UUID request id, got %q", requestID(r))
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "spoofed\r\nvalue")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if _, err := uuid.Parse(res.Header().Get("X-Request-ID")); err != nil {
		t.Fatalf("expected generated UUID request id, got %q", res.Header().Get("X-Request-ID"))
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
