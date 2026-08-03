package main

import (
	"errors"
	"net/http"

	"github.com/hypernova-banking/api/internal/auth"
)

type mcpPINRequest struct {
	PIN string `json:"pin"`
}

// mcpPINStatus exposes only the presence of a configured confirmation PIN.
// The hash and the PIN itself never cross the HTTP boundary.
func (h authHandler) mcpPINStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.MCPPINStatus(r.Context(), authenticatedUser(r))
	if err != nil {
		writeMCPPINError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// mcpPINSet creates or replaces the confirmation PIN. The route is protected
// by a valid bearer session and requireMFA in registerAuthRoutes.
func (h authHandler) mcpPINSet(w http.ResponseWriter, r *http.Request) {
	var request mcpPINRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "pin must be provided as a JSON field")
		return
	}
	if err := h.service.SetMCPPIN(r.Context(), authenticatedUser(r), request.PIN, requestMetadata(r)); err != nil {
		writeMCPPINError(w, err)
		return
	}
	status, err := h.service.MCPPINStatus(r.Context(), authenticatedUser(r))
	if err != nil {
		writeMCPPINError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func writeMCPPINError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMCPPINInvalid):
		writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_pin", "the MCP PIN must contain exactly four digits")
	case errors.Is(err, auth.ErrMCPPINNotConfigured):
		writeErrorCode(w, http.StatusConflict, "mcp_pin_not_configured", "configure an MCP PIN before confirming financial actions")
	case errors.Is(err, auth.ErrMCPPINExpired):
		writeErrorCode(w, http.StatusConflict, "mcp_pin_expired", "the MCP PIN has expired; generate a new one")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeErrorCode(w, http.StatusUnauthorized, "invalid_access_token", "invalid authenticated user")
	case errors.Is(err, auth.ErrMCPPINUnavailable):
		writeErrorCode(w, http.StatusServiceUnavailable, "mcp_pin_unavailable", "MCP PIN is unavailable")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "mcp_pin_error", "MCP PIN operation failed")
	}
}
