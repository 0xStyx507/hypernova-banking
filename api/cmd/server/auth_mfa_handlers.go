package main

import (
	"errors"
	"net/http"

	"github.com/hypernova-banking/api/internal/auth"
)

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
	if err := h.service.MarkSessionMFAVerified(r.Context(), bearerToken(r.Header.Get("Authorization")), authenticatedUser(r)); err != nil {
		writeMFAError(w, err)
		return
	}
	// Return the complete public MFA contract after verification. Keeping the
	// response aligned with MFAStatus avoids clients having to infer whether
	// enrollment actually completed from the `enabled` flag alone.
	status, err := h.service.MFAStatus(r.Context(), authenticatedUser(r))
	if err != nil {
		writeMFAError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
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
	case errors.Is(err, auth.ErrInvalidAccessToken):
		writeErrorCode(w, http.StatusUnauthorized, "invalid_access_token", "invalid access token")
	case errors.Is(err, auth.ErrMFAUnavailable):
		writeErrorCode(w, http.StatusServiceUnavailable, "mfa_unavailable", "multi-factor authentication is unavailable")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeErrorCode(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "mfa_error", "multi-factor authentication failed")
	}
}
