package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
)

// authHandler coordinates HTTP adapters for the identity module. The business
// rules remain in internal/auth; this type only translates HTTP requests and
// responses at the monolith boundary.
type authHandler struct {
	service       *auth.Service
	ledgerService *ledger.Service
}

// registerAuthRoutes exposes identity operations and provisions the default
// financial account as part of successful registration.
func registerAuthRoutes(router chi.Router, service *auth.Service, ledgerService *ledger.Service) {
	handler := authHandler{service: service, ledgerService: ledgerService}
	router.Route("/api/v1/auth", func(router chi.Router) {
		router.Post("/register", handler.register)
		router.Post("/login", handler.login)
		router.Post("/refresh", handler.refresh)
		router.Post("/logout", handler.logout)
		router.Get("/oauth/{provider}/start", handler.oauthStart)
		router.Get("/oauth/{provider}/callback", handler.oauthCallback)
		router.Post("/oauth/{provider}/exchange", handler.oauthExchange)
		router.With(ledgerAuthentication(service)).Get("/mfa", handler.mfaStatus)
		router.With(ledgerAuthentication(service)).Post("/mfa/enroll", handler.mfaEnroll)
		router.With(ledgerAuthentication(service)).Post("/mfa/verify", handler.mfaVerify)
	})
}
