// Package auth implements identity, password verification and opaque sessions.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/hypernova-banking/api/internal/audit"
)

var (
	// ErrInvalidInput is returned when a request fails domain validation.
	ErrInvalidInput = errors.New("invalid input")
	// ErrEmailInUse avoids exposing whether unrelated account data exists.
	ErrEmailInUse = errors.New("email already in use")
	// ErrInvalidCredentials is shared by missing users and wrong passwords.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidRefreshToken is deliberately generic to avoid token probing.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrInvalidAccessToken is deliberately generic to avoid session probing.
	ErrInvalidAccessToken = errors.New("invalid access token")
)

const (
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 7 * 24 * time.Hour
	defaultBcryptCost = bcrypt.DefaultCost
	maxPasswordBytes  = 72
)

// Config controls the lifetime and hashing cost of authentication artifacts.
type Config struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	BcryptCost int
	// MFAEncryptionKey encrypts TOTP secrets before they are persisted.
	// Production deployments must provide a stable 32-byte secret.
	MFAEncryptionKey []byte
	// OAuth contains provider endpoints and short-lived artifact settings.
	OAuth OAuthConfig
}

// RequestMetadata contains non-secret request context for audit records.
type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

// RegisterInput contains the public fields accepted by registration.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
}

// UpdateProfileInput contains the profile fields that can be changed without
// an email-verification workflow. Email remains immutable in this endpoint.
type UpdateProfileInput struct {
	FullName string
}

// User is the safe user representation returned by the service.
type User struct {
	ID        uuid.UUID
	Email     string
	FullName  string
	CreatedAt time.Time
}

// Tokens contains opaque credentials and their expiry timestamps.
type Tokens struct {
	User             User
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// Service coordinates identity and session operations in PostgreSQL.
type Service struct {
	pool       *pgxpool.Pool
	accessTTL  time.Duration
	refreshTTL time.Duration
	bcryptCost int
	mfaKey     []byte
	oauth      OAuthConfig
}

// NewService creates an authentication service with safe defaults for omitted
// configuration values.
func NewService(pool *pgxpool.Pool, config Config) *Service {
	if config.AccessTTL <= 0 {
		config.AccessTTL = defaultAccessTTL
	}
	if config.RefreshTTL <= 0 {
		config.RefreshTTL = defaultRefreshTTL
	}
	if config.BcryptCost < bcrypt.MinCost {
		config.BcryptCost = defaultBcryptCost
	}
	return &Service{
		pool:       pool,
		accessTTL:  config.AccessTTL,
		refreshTTL: config.RefreshTTL,
		bcryptCost: config.BcryptCost,
		mfaKey:     append([]byte(nil), config.MFAEncryptionKey...),
		oauth:      config.OAuth,
	}
}

// ValidateRegistration validates and normalizes user input without touching
// the database. It is shared by the HTTP registration flow and the seed.
func ValidateRegistration(input RegisterInput) (RegisterInput, error) {
	input.Email = NormalizeEmail(input.Email)
	input.FullName = strings.TrimSpace(input.FullName)
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil || parsedEmail.Address != input.Email || len(input.Email) > 320 {
		return RegisterInput{}, ErrInvalidInput
	}
	if !validFullName(input.FullName) {
		return RegisterInput{}, ErrInvalidInput
	}
	if len([]byte(input.Password)) < 8 || len([]byte(input.Password)) > maxPasswordBytes {
		return RegisterInput{}, ErrInvalidInput
	}
	return input, nil
}

// NormalizeEmail applies the canonical representation used by uniqueness and
// login lookups.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateLogin validates the minimum shape required before credential lookup.
// It keeps malformed requests distinct from valid requests with bad secrets.
func ValidateLogin(email, password string) (string, error) {
	email = NormalizeEmail(email)
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email || len(email) > 320 || strings.TrimSpace(password) == "" {
		return "", ErrInvalidInput
	}
	return email, nil
}

// HashPassword returns a one-way bcrypt hash. The plaintext is never stored.
func HashPassword(password string, cost int) (string, error) {
	if cost < bcrypt.MinCost {
		cost = defaultBcryptCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// Register creates an identity and records the action atomically.
func (s *Service) Register(ctx context.Context, input RegisterInput, metadata RequestMetadata) (User, error) {
	if s == nil || s.pool == nil {
		return User{}, fmt.Errorf("authentication service is not configured")
	}
	input, err := ValidateRegistration(input)
	if err != nil {
		return User{}, err
	}
	hash, err := HashPassword(input.Password, s.bcryptCost)
	if err != nil {
		return User{}, err
	}

	user := User{ID: uuid.New(), Email: input.Email, FullName: input.FullName, CreatedAt: time.Now().UTC()}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin registration: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, user.ID, user.Email, hash, user.FullName, user.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailInUse
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	if err := recordAudit(ctx, tx, &user.ID, "register", nil, metadata); err != nil {
		return User{}, fmt.Errorf("audit registration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit registration: %w", err)
	}
	return user, nil
}

// UpdateProfile changes the authenticated user's safe personal fields and
// records the change for auditability.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateProfileInput, metadata RequestMetadata) (User, error) {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return User{}, ErrInvalidInput
	}
	input.FullName = strings.TrimSpace(input.FullName)
	if !validFullName(input.FullName) {
		return User{}, ErrInvalidInput
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET full_name = $1, updated_at = NOW()
		WHERE id = $2 AND active
		RETURNING id, email, full_name, created_at
	`, input.FullName, userID).Scan(&user.ID, &user.Email, &user.FullName, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidInput
		}
		return User{}, fmt.Errorf("update profile: %w", err)
	}
	if err := recordAudit(ctx, s.pool, &userID, "profile_updated", map[string]any{"fields": []string{"full_name"}}, metadata); err != nil {
		return User{}, fmt.Errorf("audit profile update: %w", err)
	}
	return user, nil
}

// validFullName accepts international letters and combining accent marks,
// while rejecting punctuation, symbols and control characters. This keeps
// names such as "María José" and "Ñusta" valid without allowing log/header
// injection through newlines or unexpected delimiters.
func validFullName(value string) bool {
	runes := []rune(value)
	if len(runes) < 2 || len(runes) > 120 {
		return false
	}
	hasLetter := false
	for _, character := range runes {
		switch {
		case unicode.IsLetter(character):
			hasLetter = true
		case unicode.Is(unicode.Mn, character):
			// Permit decomposed accents such as e + combining acute mark.
		case character == ' ':
		default:
			return false
		}
	}
	return hasLetter
}

// Login preserves the original password-only service contract for callers
// that do not yet collect a second factor.
func (s *Service) Login(ctx context.Context, email, password string, metadata RequestMetadata) (Tokens, error) {
	return s.LoginWithMFA(ctx, email, password, "", metadata)
}

// LoginWithMFA verifies credentials, an enabled TOTP factor when required,
// and rotates a new session into existence.
func (s *Service) LoginWithMFA(ctx context.Context, email, password, totpCode string, metadata RequestMetadata) (Tokens, error) {
	if s == nil || s.pool == nil {
		return Tokens{}, fmt.Errorf("authentication service is not configured")
	}
	validatedEmail, err := ValidateLogin(email, password)
	if err != nil {
		return Tokens{}, err
	}
	email = validatedEmail
	var user User
	var passwordHash string
	var active bool
	var mfaEnabled bool
	var encryptedMFASecret []byte
	err = s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, created_at, active,
		       mfa_enabled, mfa_secret_encrypted
		FROM users WHERE LOWER(email) = $1
	`, email).Scan(&user.ID, &user.Email, &passwordHash, &user.FullName, &user.CreatedAt, &active, &mfaEnabled, &encryptedMFASecret)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		var userID *uuid.UUID
		if err == nil {
			userID = &user.ID
		}
		_ = recordAudit(ctx, s.pool, userID, "login_failure", nil, metadata)
		return Tokens{}, ErrInvalidCredentials
	}
	if mfaEnabled {
		if err := s.verifyLoginMFA(ctx, user.ID, encryptedMFASecret, totpCode, metadata); err != nil {
			return Tokens{}, err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Tokens{}, fmt.Errorf("begin login: %w", err)
	}
	defer tx.Rollback(ctx)
	tokens, err := s.createSession(ctx, tx, user, metadata, "login", mfaEnabled)
	if err != nil {
		return Tokens{}, err
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1", user.ID); err != nil {
		return Tokens{}, fmt.Errorf("update last login: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Tokens{}, fmt.Errorf("commit login: %w", err)
	}
	return tokens, nil
}

// Refresh rotates both opaque tokens while locking the session row. The old
// refresh token becomes unusable as soon as this transaction commits.
func (s *Service) Refresh(ctx context.Context, refreshToken string, metadata RequestMetadata) (Tokens, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(refreshToken) == "" {
		return Tokens{}, ErrInvalidRefreshToken
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Tokens{}, fmt.Errorf("begin refresh: %w", err)
	}
	defer tx.Rollback(ctx)

	var sessionID, userID uuid.UUID
	var refreshExpiresAt time.Time
	refreshHash := tokenHash(refreshToken)
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, refresh_expires_at
		FROM sessions
		WHERE refresh_token_hash = $1 AND revoked_at IS NULL
		FOR UPDATE
	`, refreshHash).Scan(&sessionID, &userID, &refreshExpiresAt)
	if err != nil {
		// A hash present in the history with used_at set means an old refresh
		// token was replayed. Revoke every session for the user so a stolen
		// token cannot coexist with the legitimate rotated session.
		var reusedUserID uuid.UUID
		var usedAt *time.Time
		historyErr := tx.QueryRow(ctx, `
			SELECT user_id, used_at FROM session_refresh_tokens WHERE token_hash = $1
		`, refreshHash).Scan(&reusedUserID, &usedAt)
		if historyErr == nil && usedAt != nil {
			_, _ = tx.Exec(ctx, `UPDATE sessions SET revoked_at = COALESCE(revoked_at, NOW()), last_used_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, reusedUserID)
			_ = recordAudit(ctx, tx, &reusedUserID, "refresh_token_reuse_detected", nil, metadata)
			_ = tx.Commit(ctx)
		}
		return Tokens{}, ErrInvalidRefreshToken
	}
	if time.Now().UTC().After(refreshExpiresAt) {
		return Tokens{}, ErrInvalidRefreshToken
	}

	user, err := findUser(ctx, tx, userID)
	if err != nil {
		return Tokens{}, ErrInvalidRefreshToken
	}
	accessToken, err := generateToken()
	if err != nil {
		return Tokens{}, err
	}
	newRefreshToken, err := generateToken()
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	accessExpiresAt := now.Add(s.accessTTL)
	newRefreshExpiresAt := now.Add(s.refreshTTL)
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET access_token_hash = $1, refresh_token_hash = $2,
		    access_expires_at = $3, refresh_expires_at = $4, last_used_at = $5
		WHERE id = $6
	`, tokenHash(accessToken), tokenHash(newRefreshToken), accessExpiresAt, newRefreshExpiresAt, now, sessionID); err != nil {
		return Tokens{}, fmt.Errorf("rotate session: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE session_refresh_tokens SET used_at = $1 WHERE token_hash = $2 AND session_id = $3 AND used_at IS NULL`, now, refreshHash, sessionID); err != nil {
		return Tokens{}, fmt.Errorf("mark refresh token used: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO session_refresh_tokens (token_hash, session_id, user_id, issued_at) VALUES ($1, $2, $3, $4)`, tokenHash(newRefreshToken), sessionID, userID, now); err != nil {
		return Tokens{}, fmt.Errorf("store rotated refresh token: %w", err)
	}
	if err := recordAudit(ctx, tx, &userID, "refresh", map[string]any{"session_id": sessionID.String()}, metadata); err != nil {
		return Tokens{}, fmt.Errorf("audit refresh: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Tokens{}, fmt.Errorf("commit refresh: %w", err)
	}
	return Tokens{User: user, AccessToken: accessToken, RefreshToken: newRefreshToken, AccessExpiresAt: accessExpiresAt, RefreshExpiresAt: newRefreshExpiresAt}, nil
}

// Logout revokes the session identified by an access token. Repeating logout
// is intentionally idempotent and does not reveal token validity.
func (s *Service) Logout(ctx context.Context, accessToken string, metadata RequestMetadata) error {
	if s == nil || s.pool == nil || strings.TrimSpace(accessToken) == "" {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin logout: %w", err)
	}
	defer tx.Rollback(ctx)

	var sessionID, userID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE sessions SET revoked_at = NOW(), last_used_at = NOW()
		WHERE access_token_hash = $1 AND revoked_at IS NULL
		RETURNING id, user_id
	`, tokenHash(accessToken)).Scan(&sessionID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE session_refresh_tokens SET used_at = COALESCE(used_at, NOW()) WHERE session_id = $1 AND used_at IS NULL`, sessionID); err != nil {
		return fmt.Errorf("revoke session refresh tokens: %w", err)
	}
	if err := recordAudit(ctx, tx, &userID, "logout", nil, metadata); err != nil {
		return fmt.Errorf("audit logout: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit logout: %w", err)
	}
	return nil
}

// Authenticate resolves an active bearer token to its user. Access tokens are
// opaque hashes in PostgreSQL and are never returned by this lookup.
func (s *Service) Authenticate(ctx context.Context, accessToken string) (uuid.UUID, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(accessToken) == "" {
		return uuid.Nil, ErrInvalidAccessToken
	}
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE sessions
		SET last_used_at = NOW()
		WHERE access_token_hash = $1
		  AND revoked_at IS NULL
		  AND access_expires_at > NOW()
		  AND EXISTS (SELECT 1 FROM users WHERE users.id = sessions.user_id AND users.active)
		RETURNING user_id
	`, tokenHash(accessToken)).Scan(&userID)
	if err != nil {
		return uuid.Nil, ErrInvalidAccessToken
	}
	return userID, nil
}

func (s *Service) createSession(ctx context.Context, executor audit.Executor, user User, metadata RequestMetadata, event string, mfaVerified bool) (Tokens, error) {
	accessToken, err := generateToken()
	if err != nil {
		return Tokens{}, err
	}
	refreshToken, err := generateToken()
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	accessExpiresAt := now.Add(s.accessTTL)
	refreshExpiresAt := now.Add(s.refreshTTL)
	sessionID := uuid.New()
	var mfaVerifiedAt any
	if mfaVerified {
		mfaVerifiedAt = now
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO sessions (id, user_id, access_token_hash, refresh_token_hash, access_expires_at, refresh_expires_at, mfa_verified_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9)
	`, sessionID, user.ID, tokenHash(accessToken), tokenHash(refreshToken), accessExpiresAt, refreshExpiresAt, mfaVerifiedAt, nullableIP(metadata.IPAddress), truncate(metadata.UserAgent, 512)); err != nil {
		return Tokens{}, fmt.Errorf("insert session: %w", err)
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO session_refresh_tokens (token_hash, session_id, user_id, issued_at)
		VALUES ($1, $2, $3, $4)
	`, tokenHash(refreshToken), sessionID, user.ID, now); err != nil {
		return Tokens{}, fmt.Errorf("store session refresh token: %w", err)
	}
	if err := recordAudit(ctx, executor, &user.ID, event, map[string]any{"session_id": sessionID.String()}, metadata); err != nil {
		return Tokens{}, fmt.Errorf("audit session: %w", err)
	}
	return Tokens{User: user, AccessToken: accessToken, RefreshToken: refreshToken, AccessExpiresAt: accessExpiresAt, RefreshExpiresAt: refreshExpiresAt}, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func findUser(ctx context.Context, queryer queryer, userID uuid.UUID) (User, error) {
	var user User
	err := queryer.QueryRow(ctx, `SELECT id, email, full_name, created_at FROM users WHERE id = $1 AND active`, userID).Scan(&user.ID, &user.Email, &user.FullName, &user.CreatedAt)
	return user, err
}

func recordAudit(ctx context.Context, executor audit.Executor, userID *uuid.UUID, event string, details map[string]any, metadata RequestMetadata) error {
	return audit.Record(ctx, executor, userID, event, details, metadata.IPAddress, metadata.UserAgent)
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenHash(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func nullableIP(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
