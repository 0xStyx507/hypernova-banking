package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	MFACode  string `json:"mfa_code,omitempty"`
}

type mfaCodeRequest struct {
	Code string `json:"code"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	CreatedAt time.Time `json:"created_at"`
}

type tokensResponse struct {
	User             userResponse `json:"user"`
	AccessToken      string       `json:"access_token"`
	RefreshToken     string       `json:"refresh_token"`
	AccessExpiresAt  time.Time    `json:"access_expires_at"`
	RefreshExpiresAt time.Time    `json:"refresh_expires_at"`
}

type registrationResponse struct {
	User    userResponse       `json:"user"`
	Account ledger.AccountView `json:"account"`
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
		router.With(ledgerAuthentication(service)).Get("/mfa", handler.mfaStatus)
		router.With(ledgerAuthentication(service)).Post("/mfa/enroll", handler.mfaEnroll)
		router.With(ledgerAuthentication(service)).Post("/mfa/verify", handler.mfaVerify)
	})
}

type authHandler struct {
	service       *auth.Service
	ledgerService *ledger.Service
}

func (h authHandler) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	user, err := h.service.Register(ctx, auth.RegisterInput{Email: request.Email, Password: request.Password, FullName: request.FullName}, requestMetadata(r))
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
	account, err := h.ledgerService.CreateAccount(ctx, user.ID, "HNL", ledger.RequestMetadata{IPAddress: requestMetadata(r).IPAddress, UserAgent: requestMetadata(r).UserAgent})
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
		case errors.Is(err, auth.ErrMFAUnavailable):
			writeErrorCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "multi-factor authentication is unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "login failed")
		}
		return
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

func (h authHandler) mfaStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.MFAStatus(r.Context(), authenticatedUser(r))
	if err != nil {
		writeMFAError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h authHandler) mfaEnroll(w http.ResponseWriter, r *http.Request) {
	enrollment, err := h.service.EnrollMFA(r.Context(), authenticatedUser(r), requestMetadata(r))
	if err != nil {
		writeMFAError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, enrollment)
}

func (h authHandler) mfaVerify(w http.ResponseWriter, r *http.Request) {
	var request mfaCodeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid MFA request")
		return
	}
	if err := h.service.VerifyMFA(r.Context(), authenticatedUser(r), request.Code, requestMetadata(r)); err != nil {
		writeMFAError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func writeMFAError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMFAEnrollmentRequired):
		writeErrorCode(w, http.StatusConflict, "mfa_enrollment_required", "start MFA enrollment before verifying a code")
	case errors.Is(err, auth.ErrMFAEnrollmentExpired):
		writeErrorCode(w, http.StatusGone, "mfa_enrollment_expired", "MFA enrollment has expired")
	case errors.Is(err, auth.ErrMFAAlreadyEnabled):
		writeErrorCode(w, http.StatusConflict, "mfa_already_enabled", "multi-factor authentication is already enabled")
	case errors.Is(err, auth.ErrInvalidMFACode):
		writeErrorCode(w, http.StatusUnauthorized, "mfa_invalid_code", "invalid multi-factor authentication code")
	case errors.Is(err, auth.ErrMFAUnavailable):
		writeErrorCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "multi-factor authentication is unavailable")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeErrorCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "mfa_error", "multi-factor authentication failed")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request contains trailing data")
	}
	return nil
}

func requestMetadata(r *http.Request) auth.RequestMetadata {
	address := r.RemoteAddr
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	return auth.RequestMetadata{IPAddress: address, UserAgent: r.UserAgent()}
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func userResponseFrom(user auth.User) userResponse {
	return userResponse{ID: user.ID.String(), Email: user.Email, FullName: user.FullName, CreatedAt: user.CreatedAt}
}

func tokensResponseFrom(tokens auth.Tokens) tokensResponse {
	return tokensResponse{User: userResponseFrom(tokens.User), AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, AccessExpiresAt: tokens.AccessExpiresAt, RefreshExpiresAt: tokens.RefreshExpiresAt}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	code := "internal_error"
	switch message {
	case "invalid request":
		code = "invalid_request"
	case "invalid registration data":
		code = "invalid_registration"
	case "email already in use":
		code = "email_already_in_use"
	case "invalid login data":
		code = "invalid_login"
	case "invalid credentials":
		code = "invalid_credentials"
	case "invalid refresh token":
		code = "invalid_refresh_token"
	case "authentication required":
		code = "authentication_required"
	}
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
