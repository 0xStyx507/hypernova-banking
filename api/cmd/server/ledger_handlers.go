package main

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
)

type userIDContextKey struct{}

type accountRequest struct {
	Currency string `json:"currency"`
}

type accountRenameRequest struct {
	DisplayName string `json:"display_name"`
}

type movementRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type transferRequest struct {
	SourceAccountID      string `json:"source_account_id"`
	DestinationAccountID string `json:"destination_account_id"`
	TransferType         string `json:"transfer_type"`
	ConfirmationPIN      string `json:"confirmation_pin"`
	Amount               string `json:"amount"`
	Currency             string `json:"currency"`
}

type historyResponse struct {
	Items      []ledger.HistoryView `json:"items"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

// registerLedgerRoutes adds authenticated financial routes only when the
// ledger dependency was initialized successfully during application startup.
func registerLedgerRoutes(router chi.Router, authService *auth.Service, service *ledger.Service) {
	handler := ledgerHandler{auth: authService, service: service}
	router.Route("/api/v1", func(router chi.Router) {
		router.Use(ledgerAuthentication(authService), requireMFA(authService))
		router.Post("/accounts", handler.createAccount)
		router.Get("/accounts", handler.listAccounts)
		router.Get("/accounts/{account_id}", handler.getAccount)
		router.Patch("/accounts/{account_id}", handler.renameAccount)
		// PUT remains a compatibility alias for clients deployed before the
		// account rename contract was corrected to PATCH.
		router.Put("/accounts/{account_id}", handler.renameAccount)
		router.Delete("/accounts/{account_id}", handler.deleteAccount)
		router.Get("/accounts/{account_id}/balance", handler.getBalance)
		router.Get("/accounts/{account_id}/transactions", handler.getHistory)
		router.Get("/accounts/{account_id}/transactions.csv", handler.exportHistory)
		router.Post("/accounts/{account_id}/deposits", handler.deposit)
		router.Post("/accounts/{account_id}/withdrawals", handler.withdraw)
		router.Post("/transfers", handler.transfer)
	})
}

// requireMFA is a server-side authorization boundary for account data and
// money movement. Enrollment endpoints remain available behind the base
// session middleware so a new user can activate MFA before using the ledger.
func requireMFA(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := authenticatedUser(r)
			token := bearerToken(r.Header.Get("Authorization"))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			enabled, err := service.MFAEnabled(ctx, userID)
			if err != nil {
				writeErrorCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "multi-factor authentication is unavailable")
				return
			}
			if !enabled {
				writeErrorCode(w, http.StatusForbidden, "mfa_required", "complete multi-factor authentication before accessing account data")
				return
			}
			verified, err := service.IsSessionMFAVerified(ctx, token)
			if err != nil {
				writeErrorCode(w, http.StatusUnauthorized, "invalid_access_token", "invalid access token")
				return
			}
			if !verified {
				writeErrorCode(w, http.StatusForbidden, "mfa_required", "complete multi-factor authentication before accessing account data")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ledgerAuthentication(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" {
				writeErrorCode(w, http.StatusUnauthorized, "authentication_required", "authentication required")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			userID, err := service.Authenticate(ctx, token)
			if err != nil {
				writeErrorCode(w, http.StatusUnauthorized, "invalid_access_token", "invalid access token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDContextKey{}, userID)))
		})
	}
}

type ledgerHandler struct {
	auth    *auth.Service
	service *ledger.Service
}

func (h ledgerHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var request accountRequest
	if err := decodeJSON(w, r, &request); err != nil && !errors.Is(err, io.EOF) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if strings.TrimSpace(request.Currency) == "" {
		request.Currency = "USD"
	}
	view, err := h.service.CreateAccount(r.Context(), authenticatedUser(r), request.Currency)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (h ledgerHandler) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.service.ListAccounts(r.Context(), authenticatedUser(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": accounts})
}

func (h ledgerHandler) getAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.service.Account(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h ledgerHandler) renameAccount(w http.ResponseWriter, r *http.Request) {
	var request accountRenameRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid account name")
		return
	}
	view, err := h.service.RenameAccount(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), request.DisplayName, requestMetadataForLedger(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// deleteAccount closes an account only after the ledger confirms that it has
// no available or pending funds. The TigerBeetle account remains immutable;
// PostgreSQL marks it closed so its audit history is never physically erased.
func (h ledgerHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.service.CloseAccount(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), requestMetadataForLedger(r)); err != nil {
		writeLedgerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h ledgerHandler) getBalance(w http.ResponseWriter, r *http.Request) {
	balance, err := h.service.Balance(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, balance)
}

func (h ledgerHandler) getHistory(w http.ResponseWriter, r *http.Request) {
	limit := uint32(50)
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsed == 0 || parsed > 100 {
			writeErrorCode(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = uint32(parsed)
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if value, err := strconv.ParseUint(cursor, 10, 64); err != nil || value == 0 {
			writeErrorCode(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
			return
		}
	}
	items, err := h.service.History(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), limit, cursor)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	hasMore := len(items) > int(limit)
	if hasMore {
		items = items[:limit]
	}
	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = strconv.FormatInt(items[len(items)-1].CreatedAt.UnixNano(), 10)
	}
	writeJSON(w, http.StatusOK, historyResponse{Items: items, HasMore: hasMore, NextCursor: nextCursor})
}

// exportHistory writes a bounded, ownership-checked CSV snapshot. It uses the
// same TigerBeetle-backed history query as JSON, so the export cannot invent a
// balance or expose another user's account.
func (h ledgerHandler) exportHistory(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.History(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), 100, "")
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	if len(items) > 100 {
		items = items[:100]
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="hypernova-transactions.csv"`)
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"transfer_id", "type", "direction", "amount", "currency", "created_at"}); err != nil {
		return
	}
	for _, item := range items {
		if err := writer.Write([]string{item.TransferID, item.Type, item.Direction, item.Amount, item.Currency, item.CreatedAt.Format(time.RFC3339Nano)}); err != nil {
			return
		}
	}
	writer.Flush()
}

func (h ledgerHandler) deposit(w http.ResponseWriter, r *http.Request) {
	var request movementRequest
	if !decodeMovement(w, r, &request) {
		return
	}
	if !validCurrency(request.Currency) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_currency", "only USD is supported")
		return
	}
	view, err := h.service.Deposit(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), request.Amount, idempotencyKey(r), requestMetadataForLedger(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h ledgerHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	var request movementRequest
	if !decodeMovement(w, r, &request) {
		return
	}
	if !validCurrency(request.Currency) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_currency", "only USD is supported")
		return
	}
	view, err := h.service.Withdraw(r.Context(), authenticatedUser(r), chi.URLParam(r, "account_id"), request.Amount, idempotencyKey(r), requestMetadataForLedger(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h ledgerHandler) transfer(w http.ResponseWriter, r *http.Request) {
	var request transferRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return
	}
	if !validCurrency(request.Currency) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_currency", "only USD is supported")
		return
	}
	if idempotencyKey(r) == "" {
		writeErrorCode(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key is required")
		return
	}
	scope, err := ledger.NormalizeTransferScope(request.TransferType)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_transfer_type", "transfer_type must be own or external")
		return
	}
	if err := h.service.ValidateTransferTarget(r.Context(), authenticatedUser(r), request.SourceAccountID, request.DestinationAccountID, string(scope)); err != nil {
		writeLedgerError(w, err)
		return
	}
	if scope == ledger.TransferScopeExternal {
		if strings.TrimSpace(request.ConfirmationPIN) == "" {
			writeErrorCode(w, http.StatusConflict, "external_transfer_pin_required", "a confirmation PIN is required for external transfers")
			return
		}
		if h.auth == nil {
			writeErrorCode(w, http.StatusServiceUnavailable, "mcp_pin_unavailable", "MCP PIN is unavailable")
			return
		}
		if err := h.auth.VerifyMCPPIN(r.Context(), authenticatedUser(r), request.ConfirmationPIN, requestMetadata(r)); err != nil {
			writeMCPError(w, err)
			return
		}
	}
	view, err := h.service.TransferWithScope(r.Context(), authenticatedUser(r), request.SourceAccountID, request.DestinationAccountID, request.Amount, idempotencyKey(r), string(scope), requestMetadataForLedger(r))
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func decodeMovement(w http.ResponseWriter, r *http.Request, target *movementRequest) bool {
	if err := decodeJSON(w, r, target); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid request")
		return false
	}
	if strings.TrimSpace(idempotencyKey(r)) == "" {
		writeErrorCode(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key is required")
		return false
	}
	return true
}

func authenticatedUser(r *http.Request) uuid.UUID {
	userID, _ := r.Context().Value(userIDContextKey{}).(uuid.UUID)
	return userID
}

func idempotencyKey(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}

func validCurrency(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || strings.EqualFold(value, "USD")
}

func requestMetadataForLedger(r *http.Request) ledger.RequestMetadata {
	return ledger.RequestMetadata{IPAddress: requestMetadata(r).IPAddress, UserAgent: requestMetadata(r).UserAgent}
}

func writeLedgerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalidInput):
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid financial request")
	case errors.Is(err, ledger.ErrForbidden):
		writeErrorCode(w, http.StatusForbidden, "demo_deposit_disabled", "deposit channel is disabled")
	case errors.Is(err, ledger.ErrIdempotencyConflict):
		writeErrorCode(w, http.StatusConflict, "idempotency_key_reused", "idempotency key reused with different request")
	case errors.Is(err, ledger.ErrNotFound):
		writeErrorCode(w, http.StatusNotFound, "account_not_found", "account not found")
	case errors.Is(err, ledger.ErrInsufficientFunds):
		writeErrorCode(w, http.StatusUnprocessableEntity, "insufficient_funds", "insufficient funds")
	case errors.Is(err, ledger.ErrLedgerUnavailable):
		writeErrorCode(w, http.StatusServiceUnavailable, "ledger_unavailable", "financial ledger unavailable")
	case errors.Is(err, ledger.ErrLedgerRejected):
		writeErrorCode(w, http.StatusUnprocessableEntity, "ledger_rejected", "financial operation rejected")
	case errors.Is(err, ledger.ErrConflict):
		writeErrorCode(w, http.StatusConflict, "operation_in_progress", "operation conflicts with existing state")
	case errors.Is(err, ledger.ErrAccountNotEmpty):
		writeErrorCode(w, http.StatusConflict, "account_not_empty", "the account must have a zero USD balance before it can be closed")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "financial operation failed")
	}
}
