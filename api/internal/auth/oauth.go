package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hypernova-banking/api/internal/audit"
)

var (
	// ErrOAuthNotConfigured is returned when a provider has no complete server
	// configuration. This keeps local development usable without fake OAuth.
	ErrOAuthNotConfigured   = errors.New("oauth provider is not configured")
	ErrOAuthInvalidProvider = errors.New("oauth provider is not supported")
	ErrOAuthStateInvalid    = errors.New("oauth state is invalid or expired")
	ErrOAuthDenied          = errors.New("oauth authorization was denied")
	ErrOAuthProvider        = errors.New("oauth provider request failed")
	ErrOAuthIdentity        = errors.New("oauth identity is invalid")
	ErrOAuthEmailConflict   = errors.New("oauth email is already linked to another account")
	ErrOAuthExchangeInvalid = errors.New("oauth exchange code is invalid or expired")
)

// OAuthStart creates a cryptographically random state, stores only its hash,
// and returns the provider authorization URL. State is consumed atomically by
// OAuthCallback and therefore cannot be replayed.
func (s *Service) OAuthStart(ctx context.Context, provider OAuthProvider) (OAuthAuthorization, error) {
	var err error
	provider, err = normalizeOAuthProvider(provider)
	if err != nil {
		return OAuthAuthorization{}, err
	}
	config, err := s.oauthProvider(provider)
	if err != nil {
		return OAuthAuthorization{}, err
	}
	state, err := generateToken()
	if err != nil {
		return OAuthAuthorization{}, err
	}
	expiresAt := time.Now().UTC().Add(s.oauthStateTTL())
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO oauth_states (id, provider, state_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, uuid.New(), provider, tokenHash(state), expiresAt); err != nil {
		return OAuthAuthorization{}, fmt.Errorf("store oauth state: %w", err)
	}

	query := url.Values{
		"client_id":     {config.ClientID},
		"redirect_uri":  {config.RedirectURL},
		"response_type": {"code"},
		"scope":         {config.Scope},
		"state":         {state},
	}
	if provider == OAuthProviderGoogle {
		query.Set("access_type", "online")
		query.Set("prompt", "select_account")
	}
	return OAuthAuthorization{Provider: provider, URL: config.AuthURL + "?" + query.Encode(), ExpiresAt: expiresAt, State: state}, nil
}

// OAuthCallback exchanges the provider authorization code, resolves a
// verified provider identity, and issues a short-lived application code.
func (s *Service) OAuthCallback(ctx context.Context, provider OAuthProvider, code, state, providerError string, metadata RequestMetadata) (OAuthCallbackResult, error) {
	var err error
	provider, err = normalizeOAuthProvider(provider)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	config, err := s.oauthProvider(provider)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	if err := s.consumeOAuthState(ctx, provider, state); err != nil {
		return OAuthCallbackResult{}, err
	}
	if strings.TrimSpace(providerError) != "" {
		return OAuthCallbackResult{}, ErrOAuthDenied
	}
	if strings.TrimSpace(code) == "" {
		return OAuthCallbackResult{}, ErrOAuthProvider
	}
	profile, err := fetchOAuthProfile(ctx, provider, config, code, s.oauthHTTPClient())
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	return s.createOAuthExchange(ctx, provider, profile, metadata)
}

// OAuthExchange consumes a successful callback code only after MFA, when
// enabled, has been supplied. Failed MFA attempts leave the code retryable
// until its short expiration; a successful exchange deletes it in the same
// transaction that creates the session.
func (s *Service) OAuthExchange(ctx context.Context, provider OAuthProvider, code, mfaCode string, metadata RequestMetadata) (Tokens, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(code) == "" {
		return Tokens{}, ErrOAuthExchangeInvalid
	}
	provider = OAuthProvider(strings.ToLower(strings.TrimSpace(string(provider))))
	if provider != OAuthProviderGoogle && provider != OAuthProviderGitHub {
		return Tokens{}, ErrOAuthInvalidProvider
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Tokens{}, fmt.Errorf("begin oauth exchange: %w", err)
	}
	defer tx.Rollback(ctx)

	var user User
	var expiresAt time.Time
	var mfaEnabled bool
	var encryptedMFASecret []byte
	var mfaFailedAttempts int
	var mfaLockedUntil *time.Time
	err = tx.QueryRow(ctx, `
		SELECT e.expires_at, u.id, u.email, u.full_name,
		       u.created_at, u.mfa_enabled, u.mfa_secret_encrypted,
		       u.mfa_failed_attempts, u.mfa_locked_until
		FROM oauth_exchange_codes e
		JOIN users u ON u.id = e.user_id AND u.active
		WHERE e.provider = $1 AND e.code_hash = $2
		FOR UPDATE OF e, u
	`, provider, tokenHash(code)).Scan(&expiresAt, &user.ID, &user.Email, &user.FullName, &user.CreatedAt, &mfaEnabled, &encryptedMFASecret, &mfaFailedAttempts, &mfaLockedUntil)
	if err != nil {
		return Tokens{}, ErrOAuthExchangeInvalid
	}
	if time.Now().UTC().After(expiresAt) {
		_, _ = tx.Exec(ctx, "DELETE FROM oauth_exchange_codes WHERE provider = $1 AND code_hash = $2", provider, tokenHash(code))
		_ = tx.Commit(ctx)
		return Tokens{}, ErrOAuthExchangeInvalid
	}
	if mfaEnabled {
		now := time.Now().UTC()
		if mfaLockedUntil != nil && now.Before(*mfaLockedUntil) {
			return Tokens{}, ErrMFALocked
		}
		if strings.TrimSpace(mfaCode) == "" {
			_ = audit.Record(ctx, tx, &user.ID, "oauth_mfa_required", nil, metadata.IPAddress, metadata.UserAgent)
			_ = tx.Commit(ctx)
			return Tokens{}, ErrMFARequired
		}
		secret, decryptErr := decryptMFASecret(s.mfaKey, encryptedMFASecret)
		if decryptErr != nil || !VerifyTOTP(secret, mfaCode, time.Now().UTC()) {
			mfaFailedAttempts++
			var nextLockedUntil *time.Time
			if mfaFailedAttempts >= 5 {
				lockUntil := now.Add(15 * time.Minute)
				nextLockedUntil = &lockUntil
			}
			if _, updateErr := tx.Exec(ctx, `
				UPDATE users SET mfa_failed_attempts = $1, mfa_locked_until = $2, updated_at = NOW()
				WHERE id = $3
			`, mfaFailedAttempts, nextLockedUntil, user.ID); updateErr != nil {
				return Tokens{}, fmt.Errorf("record oauth MFA failure: %w", updateErr)
			}
			_ = audit.Record(ctx, tx, &user.ID, "oauth_mfa_failure", nil, metadata.IPAddress, metadata.UserAgent)
			_ = tx.Commit(ctx)
			if nextLockedUntil != nil {
				return Tokens{}, ErrMFALocked
			}
			return Tokens{}, ErrInvalidMFACode
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET mfa_failed_attempts = 0, mfa_locked_until = NULL, updated_at = NOW()
			WHERE id = $1
		`, user.ID); err != nil {
			return Tokens{}, fmt.Errorf("reset oauth MFA failures: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM oauth_exchange_codes WHERE provider = $1 AND code_hash = $2", provider, tokenHash(code)); err != nil {
		return Tokens{}, fmt.Errorf("consume oauth exchange code: %w", err)
	}
	tokens, err := s.createSession(ctx, tx, user, metadata, "oauth_login", mfaEnabled)
	if err != nil {
		return Tokens{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tokens{}, fmt.Errorf("commit oauth exchange: %w", err)
	}
	return tokens, nil
}

func (s *Service) createOAuthExchange(ctx context.Context, provider OAuthProvider, profile oauthProfile, metadata RequestMetadata) (OAuthCallbackResult, error) {
	if s == nil || s.pool == nil {
		return OAuthCallbackResult{}, fmt.Errorf("authentication service is not configured")
	}
	validated, err := ValidateRegistration(RegisterInput{Email: profile.Email, Password: "OAuth-placeholder-1!", FullName: profile.FullName})
	if err != nil {
		return OAuthCallbackResult{}, ErrOAuthIdentity
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OAuthCallbackResult{}, fmt.Errorf("begin oauth identity: %w", err)
	}
	defer tx.Rollback(ctx)

	var user User
	var newUser bool
	var identityExists bool
	err = tx.QueryRow(ctx, `
		SELECT u.id, u.email, u.full_name, u.created_at
		FROM oauth_identities i JOIN users u ON u.id = i.user_id
		WHERE i.provider = $1 AND i.provider_subject = $2 AND u.active
	`, provider, profile.Subject).Scan(&user.ID, &user.Email, &user.FullName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingUserID uuid.UUID
		if emailErr := tx.QueryRow(ctx, "SELECT id FROM users WHERE LOWER(email) = $1", validated.Email).Scan(&existingUserID); emailErr == nil {
			return OAuthCallbackResult{}, ErrOAuthEmailConflict
		} else if !errors.Is(emailErr, pgx.ErrNoRows) {
			return OAuthCallbackResult{}, fmt.Errorf("check oauth email: %w", emailErr)
		}
		passwordHash, hashErr := newUnusablePasswordHash(s.bcryptCost)
		if hashErr != nil {
			return OAuthCallbackResult{}, hashErr
		}
		user = User{ID: uuid.New(), Email: validated.Email, FullName: validated.FullName, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, full_name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)
		`, user.ID, user.Email, passwordHash, user.FullName, user.CreatedAt); err != nil {
			if isUniqueViolation(err) {
				return OAuthCallbackResult{}, ErrOAuthEmailConflict
			}
			return OAuthCallbackResult{}, fmt.Errorf("insert oauth user: %w", err)
		}
		newUser = true
	} else if err != nil {
		return OAuthCallbackResult{}, fmt.Errorf("load oauth identity: %w", err)
	} else {
		identityExists = true
	}
	if !identityExists {
		if _, err := tx.Exec(ctx, `
			INSERT INTO oauth_identities (id, user_id, provider, provider_subject, provider_email)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New(), user.ID, provider, profile.Subject, validated.Email); err != nil {
			if isUniqueViolation(err) {
				return OAuthCallbackResult{}, ErrOAuthEmailConflict
			}
			return OAuthCallbackResult{}, fmt.Errorf("link oauth identity: %w", err)
		}
	}
	code, err := generateToken()
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	expiresAt := time.Now().UTC().Add(s.oauthExchangeTTL())
	if _, err := tx.Exec(ctx, `
		INSERT INTO oauth_exchange_codes (id, user_id, provider, code_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), user.ID, provider, tokenHash(code), expiresAt); err != nil {
		return OAuthCallbackResult{}, fmt.Errorf("store oauth exchange code: %w", err)
	}
	if err := audit.Record(ctx, tx, &user.ID, "oauth_callback", map[string]any{"provider": provider, "new_user": newUser}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return OAuthCallbackResult{}, fmt.Errorf("audit oauth callback: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OAuthCallbackResult{}, fmt.Errorf("commit oauth identity: %w", err)
	}
	return OAuthCallbackResult{User: user, ExchangeCode: code, ExpiresAt: expiresAt, NewUser: newUser}, nil
}

func (s *Service) consumeOAuthState(ctx context.Context, provider OAuthProvider, state string) error {
	if strings.TrimSpace(state) == "" {
		return ErrOAuthStateInvalid
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		DELETE FROM oauth_states
		WHERE provider = $1 AND state_hash = $2 AND expires_at > NOW()
		RETURNING id
	`, provider, tokenHash(state)).Scan(&id)
	if err != nil {
		return ErrOAuthStateInvalid
	}
	return nil
}
