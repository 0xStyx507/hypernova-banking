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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/db"
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
	startupTimeout      = 15 * time.Second
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

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), startupTimeout)
	defer cancelStartup()
	persistence, err := db.Open(startupCtx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer persistence.Close()
	if err := db.Migrate(startupCtx, persistence); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	authService := auth.NewService(persistence, auth.Config{
		AccessTTL:  durationFromEnv("AUTH_ACCESS_TTL", 15*time.Minute),
		RefreshTTL: durationFromEnv("AUTH_REFRESH_TTL", 7*24*time.Hour),
	})
	server := newHTTPServer(apiPort(os.Getenv("API_PORT")), newRouter(authService, persistence))

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
func newHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// newRouter define la superficie HTTP pequeña de fase 0. Readiness replica
// liveness intencionalmente por ahora; los chequeos de dependencias se
// agregarán junto con el módulo de base de datos en fase 1.
type readinessChecker interface {
	Ping(context.Context) error
}

// newRouter wires phase-1 identity routes while keeping health and readiness
// separate: liveness confirms the process, readiness confirms PostgreSQL.
func newRouter(authService *auth.Service, readiness readinessChecker) http.Handler {
	router := chi.NewRouter()
	router.Get(healthPath, healthHandler)
	router.Get(readinessPath, readinessHandler(readiness))
	router.Get(versionedHealthPath, healthHandler)
	if authService != nil {
		registerAuthRoutes(router, authService)
	}
	return router
}

// healthHandler devuelve una respuesta JSON estable para probes locales y del
// contenedor, sin exponer detalles internos de infraestructura.
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

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

// compile-time assertion keeps the readiness dependency explicit.
var _ readinessChecker = (*pgxpool.Pool)(nil)

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
