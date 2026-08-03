package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"

	"github.com/hypernova-banking/api/internal/audit"
)

// operationRecord is the PostgreSQL replay record associated with a
// TigerBeetle transfer. It is intentionally private: callers receive only
// OperationView values after persistence has finalized the outcome.
type operationRecord struct {
	id             uuid.UUID
	userID         uuid.UUID
	idempotencyKey string
	requestHash    []byte
	operationType  string
	transferID     string
	debitID        string
	creditID       string
	amount         int64
	currency       string
	status         string
	errorCode      string
	response       []byte
	createdAt      time.Time
	result         OperationView
}

func (s *Service) startOperation(ctx context.Context, userID uuid.UUID, key string, requestHash []byte, operationType, debitID, creditID string, amount int64, currency string) (operationRecord, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return operationRecord{}, false, fmt.Errorf("begin operation: %w", err)
	}
	defer tx.Rollback(ctx)
	accountLocks := []string{debitID, creditID}
	sort.Strings(accountLocks)
	for _, accountID := range accountLocks {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, accountID); err != nil {
			return operationRecord{}, false, fmt.Errorf("lock ledger account: %w", err)
		}
	}
	// The account records were read before this transaction began. Re-check
	// their status after acquiring the same advisory locks used by closure so a
	// stale active read cannot create a new operation on a closed account.
	systemID := systemAccountID(s.currency).String()
	for _, accountID := range accountLocks {
		if accountID == systemID {
			continue
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ledger_accounts WHERE tigerbeetle_account_id = $1 AND status = 'active')`, accountID).Scan(&active); err != nil {
			return operationRecord{}, false, fmt.Errorf("revalidate ledger account: %w", err)
		}
		if !active {
			return operationRecord{}, false, ErrNotFound
		}
	}
	operationID := uuid.New()
	transferID := tigerbeetle.ID().String()
	insertTag, err := tx.Exec(ctx, `
		INSERT INTO ledger_operations
		(id, user_id, idempotency_key, request_hash, operation_type, tigerbeetle_transfer_id, debit_account_id, credit_account_id, amount_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
	`, operationID, userID, key, requestHash, operationType, transferID, debitID, creditID, amount, currency)
	if err != nil {
		return operationRecord{}, false, fmt.Errorf("reserve operation: %w", err)
	}
	var operation operationRecord
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, idempotency_key, request_hash, operation_type, tigerbeetle_transfer_id,
		       debit_account_id, credit_account_id, amount_minor, currency, status, COALESCE(error_code, ''),
		       COALESCE(response_json, 'null'::jsonb), created_at
		FROM ledger_operations WHERE user_id = $1 AND idempotency_key = $2 FOR UPDATE
	`, userID, key).Scan(&operation.id, &operation.userID, &operation.idempotencyKey, &operation.requestHash, &operation.operationType, &operation.transferID, &operation.debitID, &operation.creditID, &operation.amount, &operation.currency, &operation.status, &operation.errorCode, &operation.response, &operation.createdAt)
	if err != nil {
		return operationRecord{}, false, fmt.Errorf("load operation: %w", err)
	}
	if !bytes.Equal(operation.requestHash, requestHash) {
		return operationRecord{}, false, ErrIdempotencyConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return operationRecord{}, false, fmt.Errorf("commit operation: %w", err)
	}
	if operation.status == "succeeded" {
		if err := json.Unmarshal(operation.response, &operation.result); err != nil {
			return operationRecord{}, true, fmt.Errorf("decode operation response: %w", err)
		}
		return operation, true, nil
	}
	if operation.status == "processing" && insertTag.RowsAffected() == 0 && operation.createdAt.After(time.Now().UTC().Add(-30*time.Second)) {
		return operation, false, ErrConflict
	}
	return operation, operation.status == "failed", nil
}

func (operation operationRecord) replayError() error {
	switch operation.errorCode {
	case "insufficient_funds":
		return ErrInsufficientFunds
	case "ledger_unavailable":
		return ErrLedgerUnavailable
	case "ledger_rejected":
		return ErrLedgerRejected
	default:
		return nil
	}
}

func (s *Service) completeOperation(ctx context.Context, operation operationRecord, metadata RequestMetadata) (OperationView, error) {
	result := OperationView{ID: operation.id.String(), Type: operation.operationType, Status: "succeeded", TransferID: operation.transferID, Amount: strconv.FormatInt(operation.amount, 10), Currency: operation.currency, CreatedAt: operation.createdAt}
	payload, err := json.Marshal(result)
	if err != nil {
		return OperationView{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OperationView{}, fmt.Errorf("begin operation completion: %w", err)
	}
	defer tx.Rollback(ctx)
	updateTag, err := tx.Exec(ctx, `UPDATE ledger_operations SET status = 'succeeded', response_json = $1, updated_at = NOW() WHERE id = $2 AND status IN ('processing', 'unknown')`, payload, operation.id)
	if err != nil {
		return OperationView{}, fmt.Errorf("store operation result: %w", err)
	}
	if updateTag.RowsAffected() == 0 {
		var status string
		var stored []byte
		if err := tx.QueryRow(ctx, "SELECT status, COALESCE(response_json, 'null'::jsonb) FROM ledger_operations WHERE id = $1", operation.id).Scan(&status, &stored); err != nil {
			return OperationView{}, fmt.Errorf("read concurrent operation result: %w", err)
		}
		if status == "succeeded" {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				return OperationView{}, fmt.Errorf("rollback concurrent operation: %w", err)
			}
			var existing OperationView
			if err := json.Unmarshal(stored, &existing); err != nil {
				return OperationView{}, fmt.Errorf("decode concurrent operation result: %w", err)
			}
			return existing, nil
		}
		return OperationView{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_transfers (tigerbeetle_transfer_id, operation_id, user_id, debit_account_id, credit_account_id, amount_minor, currency, operation_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (tigerbeetle_transfer_id) DO NOTHING
	`, operation.transferID, operation.id, operation.userID, operation.debitID, operation.creditID, operation.amount, operation.currency, operation.operationType); err != nil {
		return OperationView{}, fmt.Errorf("index operation result: %w", err)
	}
	if err := audit.Record(ctx, tx, &operation.userID, operation.operationType, map[string]any{"operation_id": operation.id.String(), "transfer_id": operation.transferID, "amount": operation.amount, "currency": operation.currency}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return OperationView{}, fmt.Errorf("audit operation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OperationView{}, fmt.Errorf("commit operation completion: %w", err)
	}
	return result, nil
}

func (s *Service) reconcileExistingTransfer(ctx context.Context, operation operationRecord, expected tigerbeetle.Transfer, metadata RequestMetadata) (OperationView, error) {
	existing, err := s.client.LookupTransfers([]tigerbeetle.Uint128{expected.ID})
	if err != nil || len(existing) != 1 {
		return s.markOperationUnknown(ctx, operation, metadata)
	}
	if !sameTransfer(existing[0], expected) {
		return s.failOperation(ctx, operation, "ledger_rejected", metadata)
	}
	return s.completeOperation(ctx, operation, metadata)
}

func sameTransfer(actual, expected tigerbeetle.Transfer) bool {
	return actual.ID == expected.ID &&
		actual.DebitAccountID == expected.DebitAccountID &&
		actual.CreditAccountID == expected.CreditAccountID &&
		actual.Amount == expected.Amount &&
		actual.PendingID == expected.PendingID &&
		actual.UserData128 == expected.UserData128 &&
		actual.UserData64 == expected.UserData64 &&
		actual.UserData32 == expected.UserData32 &&
		actual.Timeout == expected.Timeout &&
		actual.Ledger == expected.Ledger &&
		actual.Code == expected.Code &&
		actual.TransferFlags().ToUint16() == expected.TransferFlags().ToUint16()
}

func (s *Service) markOperationUnknown(ctx context.Context, operation operationRecord, metadata RequestMetadata) (OperationView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OperationView{}, fmt.Errorf("begin unknown operation: %w", err)
	}
	defer tx.Rollback(ctx)
	updateTag, err := tx.Exec(ctx, `UPDATE ledger_operations SET status = 'unknown', updated_at = NOW() WHERE id = $1 AND status IN ('processing', 'unknown')`, operation.id)
	if err != nil {
		return OperationView{}, fmt.Errorf("store unknown operation: %w", err)
	}
	if updateTag.RowsAffected() == 0 {
		return OperationView{}, ErrConflict
	}
	if err := audit.Record(ctx, tx, &operation.userID, operation.operationType+"_unknown", map[string]any{"operation_id": operation.id.String(), "transfer_id": operation.transferID}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return OperationView{}, fmt.Errorf("audit unknown operation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OperationView{}, fmt.Errorf("commit unknown operation: %w", err)
	}
	return OperationView{}, ErrLedgerUnavailable
}

func (s *Service) failOperation(ctx context.Context, operation operationRecord, code string, metadata RequestMetadata) (OperationView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OperationView{}, fmt.Errorf("begin operation failure: %w", err)
	}
	defer tx.Rollback(ctx)
	updateTag, err := tx.Exec(ctx, `UPDATE ledger_operations SET status = 'failed', error_code = $1, updated_at = NOW() WHERE id = $2 AND status IN ('processing', 'unknown')`, code, operation.id)
	if err != nil {
		return OperationView{}, fmt.Errorf("store operation failure: %w", err)
	}
	if updateTag.RowsAffected() == 0 {
		return OperationView{}, ErrConflict
	}
	if err := audit.Record(ctx, tx, &operation.userID, operation.operationType+"_failure", map[string]any{"operation_id": operation.id.String(), "error_code": code}, metadata.IPAddress, metadata.UserAgent); err != nil {
		return OperationView{}, fmt.Errorf("audit operation failure: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return OperationView{}, fmt.Errorf("commit operation failure: %w", err)
	}
	switch code {
	case "insufficient_funds":
		return OperationView{}, ErrInsufficientFunds
	case "ledger_unavailable":
		return OperationView{}, ErrLedgerUnavailable
	default:
		return OperationView{}, ErrLedgerRejected
	}
}
