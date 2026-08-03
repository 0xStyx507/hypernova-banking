// Package mcp implements the server-side approval boundary used by assistant
// and MCP clients. It never becomes a second source of financial truth.
package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hypernova-banking/api/internal/audit"
	"github.com/hypernova-banking/api/internal/ledger"
)

const (
	defaultExpiry = 5 * time.Minute
	maxReasonLen  = 280
	maxPending    = 1
)

var (
	ErrInvalidInput  = errors.New("invalid prepared action")
	ErrNotFound      = errors.New("prepared action not found")
	ErrConflict      = errors.New("prepared action state conflict")
	ErrExpired       = errors.New("prepared action expired")
	ErrCancelled     = errors.New("prepared action cancelled")
	ErrIntegrity     = errors.New("prepared action integrity failure")
	ErrPendingAction = errors.New("another prepared action is awaiting confirmation")
)

// ActionRequest contains user intent collected by an assistant. Amounts are
// integer minor units represented as strings, matching the financial API.
type ActionRequest struct {
	ActionType         string `json:"action"`
	AccountID          string `json:"account_id,omitempty"`
	SourceAccountID    string `json:"source_account_id,omitempty"`
	DestinationAccount string `json:"destination_account_id,omitempty"`
	TransferType       string `json:"transfer_type,omitempty"`
	Amount             string `json:"amount"`
	Currency           string `json:"currency"`
	Reason             string `json:"reason,omitempty"`
}

// ActionPayload is the immutable, sanitized intent stored for confirmation.
type ActionPayload struct {
	ActionType         string `json:"action"`
	AccountID          string `json:"account_id,omitempty"`
	SourceAccountID    string `json:"source_account_id,omitempty"`
	DestinationAccount string `json:"destination_account_id,omitempty"`
	TransferType       string `json:"transfer_type,omitempty"`
	Amount             string `json:"amount"`
	Currency           string `json:"currency"`
	Reason             string `json:"reason,omitempty"`
}

// Action is the public representation of a prepared operation.
type Action struct {
	ID          string                `json:"id"`
	ActionType  string                `json:"action"`
	Status      string                `json:"status"`
	Payload     ActionPayload         `json:"payload"`
	ExpiresAt   time.Time             `json:"expires_at"`
	CreatedAt   time.Time             `json:"created_at"`
	ConfirmedAt *time.Time            `json:"confirmed_at,omitempty"`
	Operation   *ledger.OperationView `json:"operation,omitempty"`
}

// Service persists approval state and delegates confirmed financial work to
// the existing ledger service.
type Service struct {
	pool   *pgxpool.Pool
	ledger *ledger.Service
}

// NewService constructs the approval service.
func NewService(pool *pgxpool.Pool, ledgerService *ledger.Service) *Service {
	return &Service{pool: pool, ledger: ledgerService}
}

// Prepare validates and stores intent without touching TigerBeetle.
func (s *Service) Prepare(ctx context.Context, userID uuid.UUID, request ActionRequest, metadata ledger.RequestMetadata) (Action, error) {
	payload, err := normalizeRequest(request)
	if err != nil || s == nil || s.pool == nil || userID == uuid.Nil {
		return Action{}, ErrInvalidInput
	}
	if !json.Valid(mustJSON(payload)) {
		return Action{}, ErrInvalidInput
	}
	actionID := uuid.New()
	expiresAt := time.Now().UTC().Add(defaultExpiry)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Action{}, fmt.Errorf("encode prepared payload: %w", err)
	}
	hash := intentHash(payload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Action{}, fmt.Errorf("begin prepared action: %w", err)
	}
	defer tx.Rollback(ctx)
	// Serialize preparation per user. The pending-action limit is a domain
	// invariant, so counting rows without a transaction-scoped lock would let
	// two concurrent requests both observe zero pending actions.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, userID.String()); err != nil {
		return Action{}, fmt.Errorf("lock prepared action owner: %w", err)
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM mcp_prepared_actions WHERE user_id = $1 AND status IN ('ready', 'confirming') AND expires_at > NOW()`, userID).Scan(&pending); err != nil {
		return Action{}, fmt.Errorf("count pending actions: %w", err)
	}
	if pending >= maxPending {
		return Action{}, ErrPendingAction
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO mcp_prepared_actions (id, user_id, action_type, payload, payload_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actionID, userID, payload.ActionType, encoded, hash[:], expiresAt)
	if err != nil {
		return Action{}, fmt.Errorf("store prepared action: %w", err)
	}
	if err := audit.Record(ctx, tx, &userID, "mcp_action_prepared", map[string]any{
		"action_id": actionID.String(), "action": payload.ActionType, "account_id": payload.AccountID,
		"source_account_id": payload.SourceAccountID, "destination_account_id": payload.DestinationAccount,
		"transfer_type": payload.TransferType, "amount": payload.Amount, "currency": payload.Currency, "reason": payload.Reason,
	}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return Action{}, fmt.Errorf("audit prepared action: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Action{}, fmt.Errorf("commit prepared action: %w", err)
	}
	return Action{ID: actionID.String(), ActionType: payload.ActionType, Status: "ready", Payload: payload, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}, nil
}

// Load returns an action only when it belongs to the authenticated user.
func (s *Service) Load(ctx context.Context, userID uuid.UUID, publicID string) (Action, error) {
	id, err := uuid.Parse(publicID)
	if err != nil || userID == uuid.Nil || s == nil || s.pool == nil {
		return Action{}, ErrNotFound
	}
	var action Action
	var payload []byte
	var operationJSON []byte
	var operationID *uuid.UUID
	var storedHash []byte
	err = s.pool.QueryRow(ctx, `
		SELECT id, action_type, status, payload, expires_at, created_at, confirmed_at, operation_id, payload_hash
		FROM mcp_prepared_actions WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&id, &action.ActionType, &action.Status, &payload, &action.ExpiresAt, &action.CreatedAt, &action.ConfirmedAt, &operationID, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Action{}, ErrNotFound
	}
	if err != nil {
		return Action{}, fmt.Errorf("load prepared action: %w", err)
	}
	if err := json.Unmarshal(payload, &action.Payload); err != nil {
		return Action{}, fmt.Errorf("decode prepared action: %w", err)
	}
	expectedHash := intentHash(action.Payload)
	if len(storedHash) > 0 && !bytes.Equal(storedHash, expectedHash[:]) {
		return Action{}, ErrIntegrity
	}
	if len(storedHash) == 0 {
		_, _ = s.pool.Exec(ctx, `UPDATE mcp_prepared_actions SET payload_hash = $1, updated_at = NOW() WHERE id = $2 AND payload_hash IS NULL`, expectedHash[:], id)
	}
	action.ID = id.String()
	if operationID != nil {
		if err := s.pool.QueryRow(ctx, `SELECT COALESCE(response_json, 'null'::jsonb) FROM ledger_operations WHERE id = $1`, *operationID).Scan(&operationJSON); err == nil && string(operationJSON) != "null" {
			var operation ledger.OperationView
			if json.Unmarshal(operationJSON, &operation) == nil {
				action.Operation = &operation
			}
		}
	}
	if action.Status == "ready" && time.Now().UTC().After(action.ExpiresAt) {
		_, _ = s.pool.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'expired', updated_at = NOW() WHERE id = $1 AND status = 'ready'`, id)
		action.Status = "expired"
	}
	return action, nil
}

// Pending returns the user's single active prepared action, if one exists.
// Clients use it after a reload so a pending confirmation cannot become an
// orphaned operation that blocks the next request.
func (s *Service) Pending(ctx context.Context, userID uuid.UUID) (Action, error) {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return Action{}, ErrNotFound
	}
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM mcp_prepared_actions
		WHERE user_id = $1 AND status IN ('ready', 'confirming') AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1
	`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Action{}, ErrNotFound
	}
	if err != nil {
		return Action{}, fmt.Errorf("load pending prepared action: %w", err)
	}
	return s.Load(ctx, userID, id.String())
}

// Claim atomically marks a ready action as confirming. Reclaiming an existing
// confirming action is safe because confirmation uses the action ID as the
// ledger idempotency key.
func (s *Service) Claim(ctx context.Context, userID uuid.UUID, publicID string) (Action, error) {
	action, err := s.Load(ctx, userID, publicID)
	if err != nil {
		return Action{}, err
	}
	if action.Status == "expired" || time.Now().UTC().After(action.ExpiresAt) {
		return Action{}, ErrExpired
	}
	if action.Status == "cancelled" {
		return Action{}, ErrCancelled
	}
	if action.Status == "confirmed" {
		return action, nil
	}
	if action.Status == "failed" {
		return Action{}, ErrConflict
	}
	parsedID, _ := uuid.Parse(action.ID)
	commandTag, err := s.pool.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'confirming', updated_at = NOW() WHERE id = $1 AND user_id = $2 AND status = 'ready' AND expires_at > NOW()`, parsedID, userID)
	if err != nil {
		return Action{}, fmt.Errorf("claim prepared action: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		// A process may have stopped after claiming but before reaching the
		// ledger. Reclaim only an old confirmation so concurrent requests still
		// receive a conflict while the original operation is active.
		commandTag, err = s.pool.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'confirming', updated_at = NOW() WHERE id = $1 AND user_id = $2 AND status = 'confirming' AND updated_at < NOW() - INTERVAL '30 seconds'`, parsedID, userID)
		if err != nil {
			return Action{}, fmt.Errorf("reclaim prepared action: %w", err)
		}
		if commandTag.RowsAffected() > 0 {
			action.Status = "confirming"
			return action, nil
		}
		current, loadErr := s.Load(ctx, userID, action.ID)
		if loadErr != nil {
			return Action{}, loadErr
		}
		if current.Status == "confirmed" {
			return current, nil
		}
		if current.Status == "expired" {
			return Action{}, ErrExpired
		}
		if current.Status == "cancelled" {
			return Action{}, ErrCancelled
		}
		return Action{}, ErrConflict
	}
	action.Status = "confirming"
	return action, nil
}

// Confirm executes the claimed intent through the financial ledger.
func (s *Service) Confirm(ctx context.Context, userID uuid.UUID, action Action, metadata ledger.RequestMetadata) (Action, error) {
	if s == nil || s.ledger == nil {
		return Action{}, ledger.ErrLedgerUnavailable
	}
	var operation ledger.OperationView
	var err error
	switch action.Payload.ActionType {
	case "deposit":
		operation, err = s.ledger.Deposit(ctx, userID, action.Payload.AccountID, action.Payload.Amount, action.ID, metadata)
	case "withdrawal":
		operation, err = s.ledger.Withdraw(ctx, userID, action.Payload.AccountID, action.Payload.Amount, action.ID, metadata)
	case "transfer":
		operation, err = s.ledger.TransferWithScope(ctx, userID, action.Payload.SourceAccountID, action.Payload.DestinationAccount, action.Payload.Amount, action.ID, action.Payload.TransferType, metadata)
	default:
		return Action{}, ErrInvalidInput
	}
	if err != nil {
		if !errors.Is(err, ledger.ErrLedgerUnavailable) {
			_, _ = s.pool.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'failed', updated_at = NOW() WHERE id = $1 AND status = 'confirming'`, uuid.MustParse(action.ID))
		}
		return Action{}, err
	}
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Action{}, fmt.Errorf("begin confirmed action: %w", err)
	}
	defer tx.Rollback(ctx)
	commandTag, err := tx.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'confirmed', operation_id = $1, confirmed_at = $2, updated_at = $2 WHERE id = $3 AND status = 'confirming'`, uuid.MustParse(operation.ID), now, uuid.MustParse(action.ID))
	if err != nil {
		return Action{}, fmt.Errorf("complete prepared action: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		_ = tx.Rollback(ctx)
		return s.Load(ctx, userID, action.ID)
	}
	if err := audit.Record(ctx, tx, &userID, "mcp_action_confirmed", map[string]any{
		"action_id": action.ID, "operation_id": operation.ID, "action": action.Payload.ActionType,
		"transfer_type": action.Payload.TransferType, "amount": action.Payload.Amount, "currency": action.Payload.Currency,
	}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return Action{}, fmt.Errorf("audit confirmed action: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Action{}, fmt.Errorf("commit confirmed action: %w", err)
	}
	action.Status = "confirmed"
	action.ConfirmedAt = &now
	action.Operation = &operation
	return action, nil
}

// Cancel invalidates only a ready action. A confirming action may represent an
// in-flight or uncertain TigerBeetle operation, so cancelling it based on a
// wall-clock lease could make the public approval state disagree with money
// that was actually moved. Claim can safely retry an old confirming action
// because the action ID is also the ledger idempotency key.
func (s *Service) Cancel(ctx context.Context, userID uuid.UUID, publicID string, metadata ledger.RequestMetadata) (Action, error) {
	action, err := s.Load(ctx, userID, publicID)
	if err != nil {
		return Action{}, err
	}
	if action.Status == "confirmed" {
		return Action{}, ErrConflict
	}
	if action.Status == "cancelled" {
		return action, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Action{}, fmt.Errorf("begin cancelled action: %w", err)
	}
	defer tx.Rollback(ctx)
	commandTag, err := tx.Exec(ctx, `UPDATE mcp_prepared_actions SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW() WHERE id = $1 AND user_id = $2 AND status = 'ready'`, uuid.MustParse(action.ID), userID)
	if err != nil {
		return Action{}, fmt.Errorf("cancel prepared action: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		_ = tx.Rollback(ctx)
		current, loadErr := s.Load(ctx, userID, action.ID)
		if loadErr != nil {
			return Action{}, loadErr
		}
		if current.Status == "cancelled" {
			return current, nil
		}
		if current.Status == "expired" {
			return Action{}, ErrExpired
		}
		return Action{}, ErrConflict
	}
	action.Status = "cancelled"
	if err := audit.Record(ctx, tx, &userID, "mcp_action_cancelled", map[string]any{"action_id": action.ID, "action": action.Payload.ActionType, "amount": action.Payload.Amount, "currency": action.Payload.Currency, "reason": action.Payload.Reason}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return Action{}, fmt.Errorf("audit cancelled action: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Action{}, fmt.Errorf("commit cancelled action: %w", err)
	}
	return action, nil
}

func normalizeRequest(request ActionRequest) (ActionPayload, error) {
	action := strings.ToLower(strings.TrimSpace(request.ActionType))
	currency := strings.ToUpper(strings.TrimSpace(request.Currency))
	if currency == "" {
		currency = "USD"
	}
	if currency != "USD" || !validAmount(request.Amount) {
		return ActionPayload{}, ErrInvalidInput
	}
	payload := ActionPayload{ActionType: action, AccountID: strings.TrimSpace(request.AccountID), SourceAccountID: strings.TrimSpace(request.SourceAccountID), DestinationAccount: strings.TrimSpace(request.DestinationAccount), TransferType: strings.TrimSpace(request.TransferType), Amount: strings.TrimSpace(request.Amount), Currency: currency, Reason: strings.TrimSpace(request.Reason)}
	if len(payload.Reason) > maxReasonLen {
		return ActionPayload{}, ErrInvalidInput
	}
	switch action {
	case "deposit", "withdrawal":
		if _, err := uuid.Parse(payload.AccountID); err != nil {
			return ActionPayload{}, ErrInvalidInput
		}
	case "transfer":
		scope, err := ledger.NormalizeTransferScope(payload.TransferType)
		if err != nil {
			return ActionPayload{}, ErrInvalidInput
		}
		payload.TransferType = string(scope)
		if _, err := uuid.Parse(payload.SourceAccountID); err != nil {
			return ActionPayload{}, ErrInvalidInput
		}
		if _, err := uuid.Parse(payload.DestinationAccount); err != nil || payload.SourceAccountID == payload.DestinationAccount {
			return ActionPayload{}, ErrInvalidInput
		}
	default:
		return ActionPayload{}, ErrInvalidInput
	}
	return payload, nil
}

func validAmount(value string) bool {
	amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && amount > 0
}

// intentHash hashes normalized fields in a fixed order so PostgreSQL JSONB
// key ordering cannot change the integrity check.
func intentHash(payload ActionPayload) [32]byte {
	value := strings.Join([]string{
		payload.ActionType,
		payload.AccountID,
		payload.SourceAccountID,
		payload.DestinationAccount,
		payload.Amount,
		payload.Currency,
		payload.TransferType,
		payload.Reason,
	}, "\x00")
	return sha256.Sum256([]byte(value))
}

func mustJSON(value ActionPayload) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
