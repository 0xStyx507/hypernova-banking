package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
	"github.com/hypernova-banking/api/internal/mcp"
)

type mcpConfirmationRequest struct {
	PIN string `json:"pin"`
}

type mcpToolCallRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mcpHandler struct {
	auth    *auth.Service
	service *mcp.Service
	ledger  *ledger.Service
}

// registerMCPRoutes exposes a deliberately small, authenticated tool surface.
// Financial tools never execute from a free-form prompt: they create a
// persisted action that must be confirmed explicitly by the caller.
func registerMCPRoutes(router chi.Router, authService *auth.Service, service *mcp.Service, ledgerService *ledger.Service) {
	if authService == nil || service == nil {
		return
	}
	handler := mcpHandler{auth: authService, service: service, ledger: ledgerService}
	router.Route("/api/v1/mcp", func(router chi.Router) {
		router.Use(ledgerAuthentication(authService), requireMFA(authService))
		router.Get("/tools", handler.tools)
		router.Post("/tools/call", handler.callTool)
		router.Post("/actions", handler.prepare)
		router.Get("/actions/pending", handler.pending)
		router.Get("/actions/{action_id}", handler.get)
		router.Post("/actions/{action_id}/confirm", handler.confirm)
		router.Post("/actions/{action_id}/cancel", handler.cancel)
	})
}

func (h mcpHandler) tools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"protocol": "hypernova-mcp-http/1",
		"tools": []map[string]any{
			{"name": "get_accounts", "read_only": true, "description": "List accounts owned by the authenticated user."},
			{"name": "get_balance", "read_only": true, "description": "Read a TigerBeetle-backed USD balance."},
			{"name": "get_transactions", "read_only": true, "description": "Read account history with cursor pagination."},
			{"name": "prepare_financial_action", "read_only": false, "description": "Prepare a deposit, withdrawal, or transfer for explicit confirmation."},
		},
	})
}

// callTool dispatches read-only tools and intent preparation. Confirmation
// and cancellation deliberately remain separate endpoints with stricter
// request shapes so a model cannot execute money movement implicitly.
func (h mcpHandler) callTool(w http.ResponseWriter, r *http.Request) {
	var request mcpToolCallRequest
	if err := decodeJSON(w, r, &request); err != nil || strings.TrimSpace(request.Name) == "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_tool_call", "invalid MCP tool call")
		return
	}
	var result any
	var err error
	userID := authenticatedUser(r)
	switch request.Name {
	case "get_accounts":
		accounts, accountErr := h.ledger.ListAccounts(r.Context(), userID)
		result, err = map[string]any{"items": accounts}, accountErr
	case "get_balance":
		var arguments struct {
			AccountID string `json:"account_id"`
		}
		if json.Unmarshal(request.Arguments, &arguments) != nil || strings.TrimSpace(arguments.AccountID) == "" {
			writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_arguments", "account_id is required")
			return
		}
		result, err = h.ledger.Balance(r.Context(), userID, arguments.AccountID)
	case "get_transactions":
		var arguments struct {
			AccountID string `json:"account_id"`
			Limit     uint32 `json:"limit"`
			Cursor    string `json:"cursor"`
		}
		if json.Unmarshal(request.Arguments, &arguments) != nil || strings.TrimSpace(arguments.AccountID) == "" {
			writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_arguments", "account_id is required")
			return
		}
		if arguments.Limit == 0 {
			arguments.Limit = 50
		}
		if arguments.Limit > 100 {
			writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_arguments", "limit must be between 1 and 100")
			return
		}
		items, historyErr := h.ledger.History(r.Context(), userID, arguments.AccountID, arguments.Limit, arguments.Cursor)
		if historyErr != nil {
			result, err = nil, historyErr
			break
		}
		hasMore := len(items) > int(arguments.Limit)
		if hasMore {
			items = items[:arguments.Limit]
		}
		nextCursor := ""
		if hasMore && len(items) > 0 {
			nextCursor = strconv.FormatInt(items[len(items)-1].CreatedAt.UnixNano(), 10)
		}
		result, err = map[string]any{"items": items, "has_more": hasMore, "next_cursor": nextCursor}, nil
	case "prepare_financial_action":
		var arguments mcp.ActionRequest
		if json.Unmarshal(request.Arguments, &arguments) != nil {
			writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_arguments", "invalid financial action arguments")
			return
		}
		result, err = h.service.Prepare(r.Context(), userID, arguments, requestMetadataForLedger(r))
	default:
		writeErrorCode(w, http.StatusNotFound, "mcp_tool_not_found", "MCP tool not found")
		return
	}
	if err != nil {
		if strings.HasPrefix(request.Name, "get_") {
			writeLedgerError(w, err)
		} else {
			writeMCPError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": request.Name, "result": result})
}

func (h mcpHandler) prepare(w http.ResponseWriter, r *http.Request) {
	var request mcp.ActionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid MCP action request")
		return
	}
	action, err := h.service.Prepare(r.Context(), authenticatedUser(r), request, requestMetadataForLedger(r))
	if err != nil {
		writeMCPError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, action)
}

func (h mcpHandler) get(w http.ResponseWriter, r *http.Request) {
	action, err := h.service.Load(r.Context(), authenticatedUser(r), chi.URLParam(r, "action_id"))
	if err != nil {
		writeMCPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (h mcpHandler) pending(w http.ResponseWriter, r *http.Request) {
	action, err := h.service.Pending(r.Context(), authenticatedUser(r))
	if errors.Is(err, mcp.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"action": nil})
		return
	}
	if err != nil {
		writeMCPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": action})
}

func (h mcpHandler) confirm(w http.ResponseWriter, r *http.Request) {
	var request mcpConfirmationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "pin is required for MCP confirmation")
		return
	}
	if h.auth == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "mcp_pin_unavailable", "MCP PIN is unavailable")
		return
	}
	action, err := h.service.Load(r.Context(), authenticatedUser(r), chi.URLParam(r, "action_id"))
	if err != nil {
		writeMCPError(w, err)
		return
	}
	if action.Status == "confirmed" {
		writeJSON(w, http.StatusOK, action)
		return
	}
	if err := h.auth.VerifyMCPPIN(r.Context(), authenticatedUser(r), request.PIN, requestMetadata(r)); err != nil {
		writeMCPError(w, err)
		return
	}
	action, err = h.service.Claim(r.Context(), authenticatedUser(r), chi.URLParam(r, "action_id"))
	if err != nil {
		writeMCPError(w, err)
		return
	}
	confirmed, err := h.service.Confirm(r.Context(), authenticatedUser(r), action, requestMetadataForLedger(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, confirmed)
}

func (h mcpHandler) cancel(w http.ResponseWriter, r *http.Request) {
	action, err := h.service.Cancel(r.Context(), authenticatedUser(r), chi.URLParam(r, "action_id"), requestMetadataForLedger(r))
	if err != nil {
		writeMCPError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func writeMCPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMCPPINLocked):
		w.Header().Set("Retry-After", "900")
		writeErrorCode(w, http.StatusTooManyRequests, "mcp_pin_locked", "MCP confirmation PIN is temporarily locked")
	case errors.Is(err, auth.ErrMCPPINInvalid):
		writeErrorCode(w, http.StatusForbidden, "mcp_pin_invalid", "invalid MCP confirmation PIN")
	case errors.Is(err, auth.ErrMCPPINNotConfigured):
		writeErrorCode(w, http.StatusConflict, "mcp_pin_not_configured", "configure an MCP PIN before confirming financial actions")
	case errors.Is(err, auth.ErrMCPPINExpired):
		writeErrorCode(w, http.StatusConflict, "mcp_pin_expired", "the MCP PIN has expired; generate a new one")
	case errors.Is(err, auth.ErrMCPPINUnavailable):
		writeErrorCode(w, http.StatusServiceUnavailable, "mcp_pin_unavailable", "MCP PIN is unavailable")
	case errors.Is(err, mcp.ErrInvalidInput):
		writeErrorCode(w, http.StatusBadRequest, "invalid_mcp_action", "invalid prepared action")
	case errors.Is(err, mcp.ErrNotFound):
		writeErrorCode(w, http.StatusNotFound, "mcp_action_not_found", "prepared action not found")
	case errors.Is(err, mcp.ErrExpired):
		writeErrorCode(w, http.StatusGone, "mcp_action_expired", "prepared action expired")
	case errors.Is(err, mcp.ErrCancelled):
		writeErrorCode(w, http.StatusConflict, "mcp_action_cancelled", "prepared action cancelled")
	case errors.Is(err, mcp.ErrPendingAction):
		writeErrorCode(w, http.StatusConflict, "mcp_action_pending", "finish or cancel the pending MCP operation before starting another one")
	case errors.Is(err, mcp.ErrConflict):
		writeErrorCode(w, http.StatusConflict, "mcp_action_conflict", "prepared action cannot change state")
	case errors.Is(err, mcp.ErrIntegrity):
		writeErrorCode(w, http.StatusInternalServerError, "mcp_action_integrity_failure", "prepared action integrity failure")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "mcp_internal_error", "MCP action failed")
	}
}
