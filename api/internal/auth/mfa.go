package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hypernova-banking/api/internal/audit"
)

const (
	mfaIssuer             = "Hypernova Banking"
	mfaSecretBytes        = 20
	mfaCodeDigits         = 6
	mfaPeriod             = 30 * time.Second
	mfaEnrollmentLifetime = 10 * time.Minute
)

var (
	// ErrMFARequired tells the client that the password was accepted but a
	// second factor is required before a session can be created.
	ErrMFARequired = errors.New("multi-factor authentication required")
	// ErrInvalidMFACode is deliberately generic to avoid revealing whether a
	// code was malformed, expired or simply incorrect.
	ErrInvalidMFACode        = errors.New("invalid multi-factor authentication code")
	ErrMFAEnrollmentRequired = errors.New("multi-factor authentication enrollment required")
	ErrMFAEnrollmentExpired  = errors.New("multi-factor authentication enrollment expired")
	ErrMFAAlreadyEnabled     = errors.New("multi-factor authentication already enabled")
	ErrMFAUnavailable        = errors.New("multi-factor authentication unavailable")
)

// MFAStatus is safe to return to an authenticated client. It never includes
// the encrypted secret or the generated provisioning URI.
type MFAStatus struct {
	Enabled  bool `json:"enabled"`
	Enrolled bool `json:"enrolled"`
}

// MFAEnrollment contains the one-time provisioning material needed to create
// a QR code in Google Authenticator, Microsoft Authenticator or any standard
// TOTP client. The secret must not be logged or persisted by the web client.
type MFAEnrollment struct {
	Secret     string    `json:"secret"`
	OTPAuthURI string    `json:"otpauth_uri"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// MFAEncryptionKey resolves the key used for AES-GCM protection of TOTP
// secrets. A configured key accepts raw 32-byte base64 or hexadecimal data.
// When omitted, a deterministic local-only key is derived from the database
// URL so a clean Compose environment remains usable; production must set the
// explicit key and keep it in a secret manager.
func MFAEncryptionKey(configured, databaseURL string) ([]byte, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		if strings.TrimSpace(databaseURL) == "" {
			return nil, ErrMFAUnavailable
		}
		hash := sha256.Sum256([]byte(databaseURL))
		return hash[:], nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(configured); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(configured); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(configured); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, fmt.Errorf("MFA encryption key must decode to 32 bytes")
}

// EnrollMFA replaces any unfinished enrollment with a fresh secret. The
// secret is encrypted before it enters PostgreSQL and is returned only in the
// immediate response so the user can scan the QR code.
func (s *Service) EnrollMFA(ctx context.Context, userID uuid.UUID, metadata RequestMetadata) (MFAEnrollment, error) {
	if s == nil || s.pool == nil || len(s.mfaKey) != 32 {
		return MFAEnrollment{}, ErrMFAUnavailable
	}
	secret, err := newMFASecret()
	if err != nil {
		return MFAEnrollment{}, err
	}
	ciphertext, err := encryptMFASecret(s.mfaKey, secret)
	if err != nil {
		return MFAEnrollment{}, err
	}
	expiresAt := time.Now().UTC().Add(mfaEnrollmentLifetime)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MFAEnrollment{}, fmt.Errorf("begin MFA enrollment: %w", err)
	}
	defer tx.Rollback(ctx)
	var accountName string
	if err := tx.QueryRow(ctx, `
		UPDATE users
		SET mfa_secret_encrypted = $1, mfa_enabled = FALSE,
		    mfa_enrollment_expires_at = $2, updated_at = NOW()
		WHERE id = $3 AND active
		RETURNING email
	`, ciphertext, expiresAt, userID).Scan(&accountName); err != nil {
		return MFAEnrollment{}, fmt.Errorf("store MFA enrollment: %w", err)
	}
	if err := audit.Record(ctx, tx, &userID, "mfa_enrollment_started", nil, metadata.IPAddress, metadata.UserAgent); err != nil {
		return MFAEnrollment{}, fmt.Errorf("audit MFA enrollment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MFAEnrollment{}, fmt.Errorf("commit MFA enrollment: %w", err)
	}
	return MFAEnrollment{Secret: secret, OTPAuthURI: otpAuthURI(secret, accountName), ExpiresAt: expiresAt}, nil
}

// MFAStatus reads only enrollment state and never exposes the protected secret.
func (s *Service) MFAStatus(ctx context.Context, userID uuid.UUID) (MFAStatus, error) {
	if s == nil || s.pool == nil {
		return MFAStatus{}, ErrMFAUnavailable
	}
	var status MFAStatus
	if err := s.pool.QueryRow(ctx, `
		SELECT mfa_enabled, mfa_secret_encrypted IS NOT NULL
		FROM users WHERE id = $1 AND active
	`, userID).Scan(&status.Enabled, &status.Enrolled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MFAStatus{}, ErrInvalidCredentials
		}
		return MFAStatus{}, fmt.Errorf("read MFA status: %w", err)
	}
	return status, nil
}

// VerifyMFA activates an enrollment only after a valid current TOTP code.
func (s *Service) VerifyMFA(ctx context.Context, userID uuid.UUID, code string, metadata RequestMetadata) error {
	if s == nil || s.pool == nil || len(s.mfaKey) != 32 {
		return ErrMFAUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MFA verification: %w", err)
	}
	defer tx.Rollback(ctx)
	var encrypted []byte
	var enabled bool
	var expiresAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT mfa_secret_encrypted, mfa_enabled, mfa_enrollment_expires_at
		FROM users WHERE id = $1 AND active FOR UPDATE
	`, userID).Scan(&encrypted, &enabled, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("load MFA enrollment: %w", err)
	}
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	if len(encrypted) == 0 {
		return ErrMFAEnrollmentRequired
	}
	if expiresAt == nil || time.Now().UTC().After(*expiresAt) {
		return ErrMFAEnrollmentExpired
	}
	secret, err := decryptMFASecret(s.mfaKey, encrypted)
	if err != nil || !VerifyTOTP(secret, code, time.Now().UTC()) {
		_ = audit.Record(ctx, tx, &userID, "mfa_verification_failure", nil, metadata.IPAddress, metadata.UserAgent)
		return ErrInvalidMFACode
	}
	if _, err := tx.Exec(ctx, `
		UPDATE users SET mfa_enabled = TRUE, mfa_enrollment_expires_at = NULL, updated_at = NOW()
		WHERE id = $1
	`, userID); err != nil {
		return fmt.Errorf("activate MFA: %w", err)
	}
	if err := audit.Record(ctx, tx, &userID, "mfa_enabled", nil, metadata.IPAddress, metadata.UserAgent); err != nil {
		return fmt.Errorf("audit MFA activation: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Service) verifyLoginMFA(ctx context.Context, userID uuid.UUID, encrypted []byte, code string, metadata RequestMetadata) error {
	if strings.TrimSpace(code) == "" {
		_ = audit.Record(ctx, s.pool, &userID, "login_mfa_required", nil, metadata.IPAddress, metadata.UserAgent)
		return ErrMFARequired
	}
	if len(s.mfaKey) != 32 {
		return ErrMFAUnavailable
	}
	secret, err := decryptMFASecret(s.mfaKey, encrypted)
	if err != nil || !VerifyTOTP(secret, code, time.Now().UTC()) {
		_ = audit.Record(ctx, s.pool, &userID, "login_mfa_failure", nil, metadata.IPAddress, metadata.UserAgent)
		return ErrInvalidMFACode
	}
	return nil
}

// VerifyTOTP implements the standard 30-second, six-digit SHA-1 TOTP profile
// used by both Google Authenticator and Microsoft Authenticator.
func VerifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != mfaCodeDigits {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return false
	}
	step := now.Unix() / int64(mfaPeriod/time.Second)
	for offset := int64(-1); offset <= 1; offset++ {
		if hmac.Equal([]byte(totpCode(key, step+offset)), []byte(code)) {
			return true
		}
	}
	return false
}

func totpCode(key []byte, counter int64) string {
	var message [8]byte
	for index := range message {
		message[len(message)-1-index] = byte(counter >> (index * 8))
	}
	hash := hmac.New(sha1.New, key) // SHA-1 is mandated by the TOTP profile.
	_, _ = hash.Write(message[:])
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", mfaCodeDigits, value%1000000)
}

func newMFASecret() (string, error) {
	bytes := make([]byte, mfaSecretBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate MFA secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}

func otpAuthURI(secret, account string) string {
	issuer := url.QueryEscape(mfaIssuer)
	return "otpauth://totp/" + issuer + ":" + url.QueryEscape(account) +
		"?secret=" + url.QueryEscape(secret) + "&issuer=" + issuer + "&algorithm=SHA1&digits=6&period=30"
}

func encryptMFASecret(key []byte, secret string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(secret), nil), nil
}

func decryptMFASecret(key, encrypted []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(encrypted) < gcm.NonceSize() {
		return "", ErrMFAUnavailable
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrMFAUnavailable
	}
	return string(plaintext), nil
}

// testTOTPCode is intentionally package-private so tests can use RFC vectors
// without making code generation part of the public API.
func testTOTPCode(secret string, now time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	return totpCode(key, now.Unix()/int64(mfaPeriod/time.Second))
}
