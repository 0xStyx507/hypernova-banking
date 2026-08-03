package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/hypernova-banking/api/internal/audit"
)

var (
	// ErrMCPPINInvalid is returned for a malformed or incorrect confirmation PIN.
	// The same error is used for all invalid values to avoid leaking validation
	// details that could help an attacker probe the confirmation factor.
	ErrMCPPINInvalid = errors.New("invalid MCP PIN")
	// ErrMCPPINNotConfigured tells the caller that a confirmation PIN must be set.
	ErrMCPPINNotConfigured = errors.New("MCP PIN is not configured")
	ErrMCPPINExpired       = errors.New("MCP PIN has expired")
	ErrMCPPINLocked        = errors.New("MCP PIN is temporarily locked")
	// ErrMCPPINUnavailable indicates that the identity store cannot serve this
	// operation. It is intentionally distinct from an invalid PIN.
	ErrMCPPINUnavailable = errors.New("MCP PIN is unavailable")
)

const (
	mcpPINLength = 4
	mcpPINTTL    = 3 * time.Minute
)

// MCPPINStatus is the non-sensitive configuration state exposed to clients.
// It never contains the PIN or its bcrypt hash.
type MCPPINStatus struct {
	Configured bool       `json:"configured"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// ValidateMCPPIN accepts exactly four ASCII digits. ASCII validation is
// intentional: a confirmation code must have one stable representation across
// browsers, mobile clients, logs and audit tooling.
func ValidateMCPPIN(pin string) error {
	if len(pin) != mcpPINLength {
		return ErrMCPPINInvalid
	}
	for _, character := range pin {
		if character < '0' || character > '9' {
			return ErrMCPPINInvalid
		}
	}
	return nil
}

// HashMCPPIN creates the one-way bcrypt representation persisted in PostgreSQL.
// Plaintext PINs must never be stored, logged or included in audit details.
func HashMCPPIN(pin string, cost int) (string, error) {
	if err := ValidateMCPPIN(pin); err != nil {
		return "", err
	}
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), cost)
	if err != nil {
		return "", fmt.Errorf("hash MCP PIN: %w", err)
	}
	return string(hash), nil
}

// MCPPINStatus returns whether the authenticated user has configured the
// confirmation factor.
func (s *Service) MCPPINStatus(ctx context.Context, userID uuid.UUID) (MCPPINStatus, error) {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return MCPPINStatus{}, ErrMCPPINUnavailable
	}
	var hash string
	var expiresAt *time.Time
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MCPPINStatus{}, fmt.Errorf("begin MCP PIN status: %w", err)
	}
	defer tx.Rollback(ctx)
	// Expiration is lazy but durable: every status read invalidates the
	// persisted factor before reporting it as unavailable to the client.
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET mcp_pin_hash = NULL, mcp_pin_expires_at = NULL,
		    mcp_pin_failed_attempts = 0, mcp_pin_locked_until = NULL,
		    updated_at = NOW()
		WHERE id = $1 AND active AND mcp_pin_expires_at IS NOT NULL
		  AND mcp_pin_expires_at <= NOW()
	`, userID); err != nil {
		return MCPPINStatus{}, fmt.Errorf("clean expired MCP PIN: %w", err)
	}
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(mcp_pin_hash, ''), mcp_pin_expires_at
		FROM users
		WHERE id = $1 AND active
	`, userID).Scan(&hash, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MCPPINStatus{}, ErrInvalidCredentials
	}
	if err != nil {
		return MCPPINStatus{}, fmt.Errorf("read MCP PIN status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MCPPINStatus{}, fmt.Errorf("commit MCP PIN status: %w", err)
	}
	return MCPPINStatus{Configured: strings.TrimSpace(hash) != "" && expiresAt != nil && expiresAt.After(time.Now().UTC()), ExpiresAt: expiresAt}, nil
}

// SetMCPPIN creates or replaces the user's MCP confirmation PIN. The HTTP
// adapter protects this operation with a valid session and a verified MFA
// factor before calling into this method.
func (s *Service) SetMCPPIN(ctx context.Context, userID uuid.UUID, pin string, metadata RequestMetadata) error {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return ErrMCPPINUnavailable
	}
	hash, err := HashMCPPIN(pin, s.bcryptCost)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MCP PIN update: %w", err)
	}
	defer tx.Rollback(ctx)

	var active bool
	if err := tx.QueryRow(ctx, `SELECT active FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("read MCP PIN owner: %w", err)
	}
	if !active {
		return ErrInvalidCredentials
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET mcp_pin_hash = $1, mcp_pin_expires_at = NOW() + $2::interval, mcp_pin_failed_attempts = 0, mcp_pin_locked_until = NULL, updated_at = NOW()
		WHERE id = $3 AND active
	`, hash, mcpPINTTL.String(), userID); err != nil {
		return fmt.Errorf("store MCP PIN: %w", err)
	}
	if err := audit.Record(ctx, tx, &userID, "mcp_pin_updated", nil, metadata.IPAddress, metadata.UserAgent); err != nil {
		return fmt.Errorf("audit MCP PIN update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MCP PIN update: %w", err)
	}
	return nil
}

// VerifyMCPPIN validates a confirmation PIN without changing the prepared
// action state. The MCP handler calls this before Claim, preventing invalid
// attempts from leaving an action stuck in the confirming state.
func (s *Service) VerifyMCPPIN(ctx context.Context, userID uuid.UUID, pin string, metadata RequestMetadata) error {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return ErrMCPPINUnavailable
	}
	if err := ValidateMCPPIN(pin); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MCP PIN verification: %w", err)
	}
	defer tx.Rollback(ctx)
	var hash string
	var failedAttempts int
	var lockedUntil *time.Time
	var expiresAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(mcp_pin_hash, ''), mcp_pin_failed_attempts, mcp_pin_locked_until, mcp_pin_expires_at
		FROM users
		WHERE id = $1 AND active
		FOR UPDATE
	`, userID).Scan(&hash, &failedAttempts, &lockedUntil, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("read MCP PIN: %w", err)
	}
	if strings.TrimSpace(hash) == "" {
		return ErrMCPPINNotConfigured
	}
	if expiresAt == nil || !expiresAt.After(time.Now().UTC()) {
		// Do not leave an expired hash usable in a later request. The cleanup is
		// part of the same row lock and transaction as verification.
		if _, clearErr := tx.Exec(ctx, `
			UPDATE users
			SET mcp_pin_hash = NULL, mcp_pin_expires_at = NULL,
			    mcp_pin_failed_attempts = 0, mcp_pin_locked_until = NULL,
			    updated_at = NOW()
			WHERE id = $1
		`, userID); clearErr != nil {
			return fmt.Errorf("clean expired MCP PIN: %w", clearErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return fmt.Errorf("commit expired MCP PIN cleanup: %w", commitErr)
		}
		return ErrMCPPINExpired
	}
	if lockedUntil != nil && time.Now().UTC().Before(*lockedUntil) {
		return ErrMCPPINLocked
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin)) != nil {
		failedAttempts++
		var nextLock *time.Time
		if failedAttempts >= 5 {
			lockUntil := time.Now().UTC().Add(15 * time.Minute)
			nextLock = &lockUntil
		}
		if _, updateErr := tx.Exec(ctx, `
			UPDATE users
			SET mcp_pin_failed_attempts = $1, mcp_pin_locked_until = $2, updated_at = NOW()
			WHERE id = $3
		`, failedAttempts, nextLock, userID); updateErr != nil {
			return fmt.Errorf("record MCP PIN failure: %w", updateErr)
		}
		_ = audit.Record(ctx, tx, &userID, "mcp_pin_verification_failure", nil, metadata.IPAddress, metadata.UserAgent)
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return fmt.Errorf("commit MCP PIN failure: %w", commitErr)
		}
		return ErrMCPPINInvalid
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET mcp_pin_failed_attempts = 0, mcp_pin_locked_until = NULL, updated_at = NOW()
		WHERE id = $1
	`, userID); err != nil {
		return fmt.Errorf("reset MCP PIN failures: %w", err)
	}
	if err := audit.Record(ctx, tx, &userID, "mcp_pin_verified", nil, metadata.IPAddress, metadata.UserAgent); err != nil {
		return fmt.Errorf("audit MCP PIN verification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MCP PIN verification: %w", err)
	}
	return nil
}
