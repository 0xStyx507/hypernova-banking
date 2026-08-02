// Package main exposes the Hypernova HTTP API and the container healthcheck.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"

	"github.com/hypernova-banking/api/internal/assistant"
	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/db"
	"github.com/hypernova-banking/api/internal/ledger"
	"github.com/hypernova-banking/api/internal/mcp"
)

// healthResponse is the stable public shape used by process probes.
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
	ledgerAddress := os.Getenv("TIGERBEETLE_ADDRESS")
	if ledgerAddress == "" {
		ledgerAddress = "127.0.0.1:3000"
	}
	resolvedLedgerAddress, err := resolveLedgerAddress(ledgerAddress)
	if err != nil {
		logger.Error("ledger address resolution failed", "error", err)
		os.Exit(1)
	}
	ledgerClient, err := tigerbeetle.NewClient(types.ToUint128(0), []string{resolvedLedgerAddress})
	if err != nil {
		logger.Error("ledger client initialization failed", "error", err)
		os.Exit(1)
	}
	defer ledgerClient.Close()
	ledgerService := ledger.NewService(persistence, ledgerClient, ledger.Config{
		AllowDemoDeposits: boolFromEnv("LEDGER_ALLOW_DEMO_DEPOSITS", false),
	})
	if err := ledgerService.EnsureSystemAccount(startupCtx); err != nil {
		logger.Error("ledger system account initialization failed", "error", err)
		os.Exit(1)
	}

	mfaKey, err := auth.MFAEncryptionKey(os.Getenv("MFA_ENCRYPTION_KEY"), os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("MFA encryption key initialization failed", "error", err)
		os.Exit(1)
	}
	authService := auth.NewService(persistence, auth.Config{
		AccessTTL:        durationFromEnv("AUTH_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       durationFromEnv("AUTH_REFRESH_TTL", 7*24*time.Hour),
		MFAEncryptionKey: mfaKey,
	})
	mcpService := mcp.NewService(persistence, ledgerService)
	mcpClient := mcp.NewClient(mcp.ClientConfig{BaseURL: "http://127.0.0.1:" + apiPort(os.Getenv("API_PORT")) + "/api/v1/mcp"})
	assistantService := assistant.NewService(assistant.LocalProvider{}, mcpClient)
	server := newHTTPServer(apiPort(os.Getenv("API_PORT")), newRouterWithServices(authService, dependencyReadiness{database: persistence, ledger: ledgerService}, ledgerService, mcpService, assistantService))

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

// resolveLedgerAddress converts a Compose DNS name into an IP because the
// native TigerBeetle client accepts numeric cluster addresses only.
func resolveLedgerAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("invalid ledger address: %w", err)
	}
	if parsed := net.ParseIP(host); parsed != nil {
		return net.JoinHostPort(parsed.String(), port), nil
	}
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("resolve ledger host %q: %w", host, err)
	}
	return net.JoinHostPort(ips[0], port), nil
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

// newRouter keeps the small health surface available to tests while the
// production constructor adds ledger, MCP and assistant dependencies.
type readinessChecker interface {
	Ping(context.Context) error
}

// newRouterWithServices wires identity, ledger, MCP and assistant routes while
// keeping health and readiness separate.
func newRouter(authService *auth.Service, readiness readinessChecker, ledgerService *ledger.Service, mcpServices ...*mcp.Service) http.Handler {
	var mcpService *mcp.Service
	if len(mcpServices) > 0 {
		mcpService = mcpServices[0]
	}
	return newRouterWithServices(authService, readiness, ledgerService, mcpService, nil)
}

func newRouterWithServices(authService *auth.Service, readiness readinessChecker, ledgerService *ledger.Service, mcpService *mcp.Service, assistantService *assistant.Service) http.Handler {
	router := chi.NewRouter()
	router.Use(newRateLimiter(rateLimitFromEnv()).middleware)
	router.Get(healthPath, healthHandler)
	router.Get(readinessPath, readinessHandler(readiness))
	router.Get(versionedHealthPath, healthHandler)
	if authService != nil {
		registerAuthRoutes(router, authService, ledgerService)
	}
	if authService != nil && ledgerService != nil {
		registerLedgerRoutes(router, authService, ledgerService)
	}
	if authService != nil && mcpService != nil {
		registerMCPRoutes(router, authService, mcpService, ledgerService)
	}
	if authService != nil && assistantService != nil {
		registerAssistantRoutes(router, authService, assistantService)
	}
	return router
}

// dependencyReadiness exposes only dependency availability, keeping internal
// database and ledger details out of the public probe response.
type dependencyReadiness struct {
	database readinessChecker
	ledger   interface{ Ping(context.Context) error }
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

func boolFromEnv(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
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
