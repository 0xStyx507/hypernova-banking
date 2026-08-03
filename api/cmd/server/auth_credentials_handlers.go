package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
)

func (h authHandler) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	metadata := requestMetadata(r)
	user, err := h.service.Register(ctx, auth.RegisterInput{Email: request.Email, Password: request.Password, FullName: request.FullName}, metadata)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid registration data")
		case errors.Is(err, auth.ErrEmailInUse):
			writeError(w, http.StatusConflict, "email already in use")
		default:
			writeError(w, http.StatusInternalServerError, "registration failed")
		}
		return
	}
	if h.ledgerService == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "ledger_unavailable", "financial ledger unavailable")
		return
	}
	account, err := h.ledgerService.CreateAccount(ctx, user.ID, "USD", ledger.RequestMetadata{IPAddress: metadata.IPAddress, UserAgent: metadata.UserAgent})
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, registrationResponse{User: userResponseFrom(user), Account: account})
}

func (h authHandler) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tokens, err := h.service.LoginWithMFA(ctx, request.Email, request.Password, request.MFACode, requestMetadata(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid login data")
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid credentials")
		case errors.Is(err, auth.ErrMFARequired):
			writeErrorCode(w, http.StatusUnauthorized, "mfa_required", "multi-factor authentication code required")
		case errors.Is(err, auth.ErrInvalidMFACode):
			writeErrorCode(w, http.StatusUnauthorized, "mfa_invalid_code", "invalid multi-factor authentication code")
		case errors.Is(err, auth.ErrMFALocked):
			w.Header().Set("Retry-After", "900")
			writeErrorCode(w, http.StatusTooManyRequests, "mfa_locked", "multi-factor authentication is temporarily locked")
		case errors.Is(err, auth.ErrMFAUnavailable):
			writeErrorCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "multi-factor authentication is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "login failed")
		}
		return
	}
	if h.ledgerService != nil {
		metadata := requestMetadata(r)
		if _, accountErr := h.ledgerService.EnsureInitialAccount(ctx, tokens.User.ID, ledger.RequestMetadata{IPAddress: metadata.IPAddress, UserAgent: metadata.UserAgent}); accountErr != nil {
			writeLedgerError(w, accountErr)
			return
		}
	}
	writeJSON(w, http.StatusOK, tokensResponseFrom(tokens))
}

func (h authHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tokens, err := h.service.Refresh(ctx, request.RefreshToken, requestMetadata(r))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	writeJSON(w, http.StatusOK, tokensResponseFrom(tokens))
}

func (h authHandler) logout(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := h.service.Logout(ctx, token, requestMetadata(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
