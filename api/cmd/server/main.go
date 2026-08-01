// Package main exposes the small phase-0 HTTP surface and the container
// healthcheck command for the Hypernova API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

// healthResponse is the stable public shape used by all phase-0 probes.
type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

const (
	defaultAPIPort      = "8080"
	healthPath          = "/healthz"
	readinessPath       = "/readyz"
	versionedHealthPath = "/api/v1/health"
	serviceName         = "hypernova-api"
	healthcheckCommand  = "healthcheck"
)

// main inicia la API HTTP o ejecuta el comando de healthcheck del contenedor.
// Mantener el healthcheck dentro del binario evita añadir curl o wget a la
// imagen runtime pequeña.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if isHealthcheckCommand(os.Args) {
		if err := runHealthcheck(); err != nil {
			logger.Error("healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	server := newHTTPServer(apiPort(os.Getenv("API_PORT")))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
		}
	}()

	logger.Info("api listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("api stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

// isHealthcheckCommand indica si el proceso fue iniciado por el healthcheck
// del contenedor en lugar de ejecutarse como servidor HTTP permanente.
func isHealthcheckCommand(args []string) bool {
	return len(args) > 1 && args[1] == healthcheckCommand
}

// apiPort devuelve el puerto configurado o el valor seguro por defecto.
func apiPort(configuredPort string) string {
	if configuredPort == "" {
		return defaultAPIPort
	}
	return configuredPort
}

// newHTTPServer construye el servidor con timeouts explícitos en cada límite.
// Esto evita que clientes lentos o incompletos retengan conexiones indefinidamente.
func newHTTPServer(port string) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// newRouter define la superficie HTTP pequeña de fase 0. Readiness replica
// liveness intencionalmente por ahora; los chequeos de dependencias se
// agregarán junto con el módulo de base de datos en fase 1.
func newRouter() http.Handler {
	router := chi.NewRouter()
	router.Get(healthPath, healthHandler)
	router.Get(readinessPath, healthHandler)
	router.Get(versionedHealthPath, healthHandler)
	return router
}

// healthHandler devuelve una respuesta JSON estable para probes locales y del
// contenedor, sin exponer detalles internos de infraestructura.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Service: serviceName})
}

// runHealthcheck verifica que el servidor acepte solicitudes desde dentro del
// contenedor y está limitado intencionalmente por un timeout corto.
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
