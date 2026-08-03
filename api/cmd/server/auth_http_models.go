package main

import (
	"time"

	"github.com/hypernova-banking/api/internal/ledger"
)

// Request models are kept next to the HTTP adapter so domain packages do not
// depend on transport-specific JSON tags.
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

type updateProfileRequest struct {
	FullName string `json:"full_name"`
}

type oauthExchangeRequest struct {
	Code    string `json:"code"`
	MFACode string `json:"mfa_code,omitempty"`
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

type oauthCallbackResponse struct {
	User         userResponse        `json:"user"`
	ExchangeCode string              `json:"exchange_code"`
	ExpiresAt    time.Time           `json:"expires_at"`
	Account      *ledger.AccountView `json:"account,omitempty"`
}
