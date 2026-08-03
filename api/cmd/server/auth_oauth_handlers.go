package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/auth"
	"github.com/hypernova-banking/api/internal/ledger"
)

func (h authHandler) oauthStart(w http.ResponseWriter, r *http.Request) {
	provider := auth.OAuthProvider(chi.URLParam(r, "provider"))
	returnTo := strings.TrimSpace(r.URL.Query().Get("return_to"))
	if returnTo != "" && !oauthRedirectAllowed(returnTo) {
		writeErrorCode(w, http.StatusBadRequest, "oauth_invalid_redirect", "OAuth redirect is not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	authorization, err := h.service.OAuthStart(ctx, provider)
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	secureCookie := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	cookiePath := "/api/v1/auth/oauth/" + string(authorization.Provider)
	http.SetCookie(w, oauthCookie("hypernova_oauth_state", authorization.State, cookiePath, authorization.ExpiresAt, secureCookie))
	if returnTo != "" {
		http.SetCookie(w, oauthCookie("hypernova_oauth_return_to", url.QueryEscape(returnTo), cookiePath, authorization.ExpiresAt, secureCookie))
	}
	http.Redirect(w, r, authorization.URL, http.StatusFound)
}

func (h authHandler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := auth.OAuthProvider(chi.URLParam(r, "provider"))
	returnTo := ""
	if cookie, err := r.Cookie("hypernova_oauth_return_to"); err == nil {
		returnTo, _ = url.QueryUnescape(cookie.Value)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	secureCookie := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	cookiePath := "/api/v1/auth/oauth/" + string(provider)
	http.SetCookie(w, oauthCookie("hypernova_oauth_state", "", cookiePath, time.Now(), secureCookie))
	http.SetCookie(w, oauthCookie("hypernova_oauth_return_to", "", cookiePath, time.Now(), secureCookie))
	state := r.URL.Query().Get("state")
	cookie, cookieErr := r.Cookie("hypernova_oauth_state")
	if cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		// Consume the presented state when possible so a mismatched browser
		// cannot leave a valid authorization response replayable.
		_, _ = h.service.OAuthCallback(ctx, provider, "", state, "state_cookie_mismatch", requestMetadata(r))
		writeErrorCode(w, http.StatusBadRequest, "oauth_invalid_state", "OAuth state is invalid or expired")
		return
	}
	result, err := h.service.OAuthCallback(ctx, provider, r.URL.Query().Get("code"), state, r.URL.Query().Get("error"), requestMetadata(r))
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	response := oauthCallbackResponse{User: userResponseFrom(result.User), ExchangeCode: result.ExchangeCode, ExpiresAt: result.ExpiresAt}
	if result.NewUser {
		if h.ledgerService == nil {
			writeErrorCode(w, http.StatusServiceUnavailable, "ledger_unavailable", "financial ledger unavailable")
			return
		}
		metadata := requestMetadata(r)
		account, accountErr := h.ledgerService.CreateAccount(ctx, result.User.ID, "USD", ledger.RequestMetadata{IPAddress: metadata.IPAddress, UserAgent: metadata.UserAgent})
		if accountErr != nil {
			writeLedgerError(w, accountErr)
			return
		}
		response.Account = &account
	}
	if returnTo != "" {
		target, parseErr := url.Parse(returnTo)
		if parseErr == nil {
			query := target.Query()
			query.Set("oauth_provider", string(provider))
			query.Set("oauth_code", result.ExchangeCode)
			target.RawQuery = query.Encode()
			http.Redirect(w, r, target.String(), http.StatusSeeOther)
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// oauthCookie centralizes the security attributes for short-lived OAuth
// browser state and keeps deletion behavior consistent with creation.
func oauthCookie(name, value, path string, expiresAt time.Time, secure bool) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if value == "" {
		maxAge = -1
	}
	return &http.Cookie{Name: name, Value: value, Path: path, MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode}
}

// oauthRedirectAllowed keeps the callback from becoming an open redirect.
// Deployments must explicitly list the web origin and/or mobile deep link.
func oauthRedirectAllowed(candidate string) bool {
	for _, configured := range strings.Split(os.Getenv("OAUTH_ALLOWED_REDIRECT_URIS"), ",") {
		if strings.TrimSpace(configured) == candidate {
			return true
		}
	}
	return false
}

func (h authHandler) oauthExchange(w http.ResponseWriter, r *http.Request) {
	var request oauthExchangeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_request", "invalid OAuth exchange request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tokens, err := h.service.OAuthExchange(ctx, auth.OAuthProvider(chi.URLParam(r, "provider")), request.Code, request.MFACode, requestMetadata(r))
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokensResponseFrom(tokens))
}

func writeOAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrOAuthInvalidProvider):
		writeErrorCode(w, http.StatusBadRequest, "oauth_invalid_provider", "OAuth provider is not supported")
	case errors.Is(err, auth.ErrOAuthNotConfigured):
		writeErrorCode(w, http.StatusServiceUnavailable, "oauth_not_configured", "OAuth provider is not configured")
	case errors.Is(err, auth.ErrOAuthStateInvalid):
		writeErrorCode(w, http.StatusBadRequest, "oauth_invalid_state", "OAuth state is invalid or expired")
	case errors.Is(err, auth.ErrOAuthDenied):
		writeErrorCode(w, http.StatusUnauthorized, "oauth_denied", "OAuth authorization was denied")
	case errors.Is(err, auth.ErrOAuthEmailConflict):
		writeErrorCode(w, http.StatusConflict, "oauth_email_conflict", "OAuth email is already linked to another account")
	case errors.Is(err, auth.ErrOAuthIdentity):
		writeErrorCode(w, http.StatusUnauthorized, "oauth_invalid_identity", "OAuth identity could not be verified")
	case errors.Is(err, auth.ErrOAuthExchangeInvalid):
		writeErrorCode(w, http.StatusUnauthorized, "oauth_invalid_exchange", "OAuth exchange code is invalid or expired")
	case errors.Is(err, auth.ErrOAuthProvider):
		writeErrorCode(w, http.StatusBadGateway, "oauth_provider_error", "OAuth provider request failed")
	case errors.Is(err, auth.ErrMFARequired):
		writeErrorCode(w, http.StatusUnauthorized, "mfa_required", "multi-factor authentication code required")
	case errors.Is(err, auth.ErrInvalidMFACode):
		writeErrorCode(w, http.StatusUnauthorized, "mfa_invalid_code", "invalid multi-factor authentication code")
	default:
		writeErrorCode(w, http.StatusInternalServerError, "oauth_error", "OAuth authentication failed")
	}
}
