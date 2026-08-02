package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/hypernova-banking/api/internal/auth"
)

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
	return auth.RequestMetadata{IPAddress: clientAddressFromRequest(r), UserAgent: r.UserAgent()}
}

// clientAddress accepts forwarded client data only when the immediate peer is
// a private/loopback proxy address. Direct public callers cannot spoof audit
// or rate-limit identity through an arbitrary X-Forwarded-For header.
func clientAddressFromRequest(r *http.Request) string {
	address := r.RemoteAddr
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	if ip := net.ParseIP(address); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			if first := strings.TrimSpace(strings.Split(forwarded, ",")[0]); net.ParseIP(first) != nil {
				return first
			}
		}
	}
	return address
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
	writeErrorCode(w, status, code, message)
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
