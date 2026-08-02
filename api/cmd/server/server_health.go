package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	healthPath          = "/healthz"
	readinessPath       = "/readyz"
	versionedHealthPath = "/api/v1/health"
	serviceName         = "hypernova-api"
)

// healthResponse is the stable public shape used by process probes.
type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// dependencyReadiness exposes only dependency availability, keeping internal
// database and ledger details out of the public probe response.
type dependencyReadiness struct {
	database readinessChecker
	ledger   readinessChecker
}

func (readiness dependencyReadiness) Ping(ctx context.Context) error {
	if readiness.database == nil || readiness.database.Ping(ctx) != nil {
		return errors.New("database unavailable")
	}
	if readiness.ledger == nil || readiness.ledger.Ping(ctx) != nil {
		return errors.New("ledger unavailable")
	}
	return nil
}

// healthHandler returns a stable JSON response without exposing infrastructure
// details to local or container probes.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Service: serviceName})
}

func readinessHandler(readiness readinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if readiness == nil || readiness.Ping(r.Context()) != nil {
			writeError(w, http.StatusServiceUnavailable, "service not ready")
			return
		}
		healthHandler(w, r)
	}
}

// runHealthcheck verifies that the server accepts requests from inside the
// container and intentionally uses a short timeout.
func runHealthcheck() error {
	port := apiPort(os.Getenv("API_PORT"))
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s%s", port, healthPath))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	return nil
}
