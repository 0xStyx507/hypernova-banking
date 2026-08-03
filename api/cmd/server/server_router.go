package main

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/assistant"
	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
	"github.com/hypernova-banking/api/internal/mcp"
)

// readinessChecker keeps router tests independent from concrete database and
// ledger clients.
type readinessChecker interface {
	Ping(context.Context) error
}

// newRouter keeps the small health surface available to tests while the
// production constructor adds ledger, MCP and assistant dependencies.
func newRouter(authService *auth.Service, readiness readinessChecker, ledgerService *ledger.Service, mcpServices ...*mcp.Service) http.Handler {
	var mcpService *mcp.Service
	if len(mcpServices) > 0 {
		mcpService = mcpServices[0]
	}
	return newRouterWithServices(authService, readiness, ledgerService, mcpService, nil)
}

// newRouterWithServices composes the modules of the monolith. Each module
// registers its own routes and remains isolated behind its service boundary.
func newRouterWithServices(authService *auth.Service, readiness readinessChecker, ledgerService *ledger.Service, mcpService *mcp.Service, assistantService *assistant.Service) http.Handler {
	router := chi.NewRouter()
	router.Use(requestIDMiddleware)
	router.Use(requestLogMiddleware)
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
