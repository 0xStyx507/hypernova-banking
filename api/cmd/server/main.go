// Package main exposes the Hypernova HTTP API and the container healthcheck.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"

	"github.com/hypernova-banking/api/internal/assistant"
	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/db"
	"github.com/hypernova-banking/api/internal/ledger"
	"github.com/hypernova-banking/api/internal/mcp"
)

// main boots the modular monolith and owns only process lifecycle concerns.
// HTTP routing, probes and environment parsing live in dedicated adapters.
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
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" {
		appEnv = "development"
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if appEnv != "development" && strings.Contains(databaseURL, "change-me-local-only") {
		logger.Error("a non-development environment cannot use the local database password")
		os.Exit(1)
	}
	persistence, err := db.Open(startupCtx, databaseURL)
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
	ledgerClient, err := tigerbeetle.NewClient(tigerbeetle.ToUint128(0), []string{resolvedLedgerAddress})
	if err != nil {
		logger.Error("ledger client initialization failed", "error", err)
		os.Exit(1)
	}
	defer ledgerClient.Close()
	allowDemoDeposits := boolFromEnv("LEDGER_ALLOW_DEMO_DEPOSITS", false)
	if appEnv != "development" && allowDemoDeposits {
		logger.Error("unsafe demo deposits are not allowed outside development", "app_env", appEnv)
		os.Exit(1)
	}
	ledgerService := ledger.NewService(persistence, ledgerClient, ledger.Config{
		AllowDemoDeposits: allowDemoDeposits,
	})
	if err := ledgerService.EnsureSystemAccount(startupCtx); err != nil {
		logger.Error("ledger system account initialization failed", "error", err)
		os.Exit(1)
	}
	if err := ledgerService.ReconcileActiveAccounts(startupCtx); err != nil {
		logger.Error("ledger account reconciliation failed", "error", err)
		os.Exit(1)
	}

	configuredMFAKey := strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY"))
	if appEnv != "development" && configuredMFAKey == "" {
		logger.Error("MFA_ENCRYPTION_KEY is required outside development")
		os.Exit(1)
	}
	mfaKey, err := auth.MFAEncryptionKey(configuredMFAKey, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("MFA encryption key initialization failed", "error", err)
		os.Exit(1)
	}
	authService := auth.NewService(persistence, auth.Config{
		AccessTTL:        durationFromEnv("AUTH_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       durationFromEnv("AUTH_REFRESH_TTL", 7*24*time.Hour),
		MFAEncryptionKey: mfaKey,
		OAuth:            auth.OAuthConfigFromEnv(),
	})
	mcpService := mcp.NewService(persistence, ledgerService)
	mcpClient := mcp.NewClient(mcp.ClientConfig{BaseURL: "http://127.0.0.1:" + apiPort(os.Getenv("API_PORT")) + "/api/v1/mcp"})
	assistantService := assistant.NewService(assistant.ProviderFromEnv(), mcpClient)
	server := newHTTPServer(apiPort(os.Getenv("API_PORT")), newRouterWithServices(authService, dependencyReadiness{
		database: persistence,
		ledger:   ledgerService,
	}, ledgerService, mcpService, assistantService))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go shutdownOnContext(ctx, server, logger)

	logger.Info("api listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("api stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

// shutdownOnContext gives in-flight requests a bounded window to finish.
func shutdownOnContext(ctx context.Context, server serverLifecycle, logger *slog.Logger) {
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}
}
