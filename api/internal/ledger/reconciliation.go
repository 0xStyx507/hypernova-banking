package ledger

import (
	"context"
	"errors"
	"fmt"

	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
)

const reconciliationBatchSize = 500

// OperationReconciliationReport describes the durable recovery work done at
// startup. Operations that TigerBeetle cannot resolve remain unknown and are
// intentionally not treated as failed.
type OperationReconciliationReport struct {
	StaleMarked  int
	Completed    int
	Failed       int
	StillUnknown int
}

// ReconcilePendingOperations resolves interrupted ledger calls from the
// TigerBeetle transfer identity already reserved in PostgreSQL. It never
// creates a replacement transfer, so a retry cannot duplicate money movement.
func (s *Service) ReconcilePendingOperations(ctx context.Context) (OperationReconciliationReport, error) {
	if s == nil || s.pool == nil || s.client == nil {
		return OperationReconciliationReport{}, ErrLedgerUnavailable
	}

	var report OperationReconciliationReport
	staleTag, err := s.pool.Exec(ctx, `
		UPDATE ledger_operations
		SET status = 'unknown', updated_at = NOW()
		WHERE status = 'processing' AND created_at < NOW() - INTERVAL '30 seconds'
	`)
	if err != nil {
		return report, fmt.Errorf("mark stale ledger operations: %w", err)
	}
	report.StaleMarked = int(staleTag.RowsAffected())

	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, idempotency_key, request_hash, operation_type, tigerbeetle_transfer_id,
		       debit_account_id, credit_account_id, amount_minor, currency, status, COALESCE(error_code, ''),
		       COALESCE(response_json, 'null'::jsonb), created_at
		FROM ledger_operations
		WHERE status = 'unknown'
		ORDER BY updated_at ASC
		LIMIT $1
	`, reconciliationBatchSize)
	if err != nil {
		return report, fmt.Errorf("list unknown ledger operations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var operation operationRecord
		if err := rows.Scan(&operation.id, &operation.userID, &operation.idempotencyKey, &operation.requestHash, &operation.operationType, &operation.transferID, &operation.debitID, &operation.creditID, &operation.amount, &operation.currency, &operation.status, &operation.errorCode, &operation.response, &operation.createdAt); err != nil {
			return report, fmt.Errorf("scan unknown ledger operation: %w", err)
		}
		if operation.amount <= 0 {
			return report, fmt.Errorf("unknown ledger operation %s has invalid amount", operation.id)
		}

		transferID, err := tigerbeetle.HexStringToUint128(operation.transferID)
		if err != nil {
			return report, fmt.Errorf("parse unknown transfer %s: %w", operation.id, err)
		}
		debitID, err := tigerbeetle.HexStringToUint128(operation.debitID)
		if err != nil {
			return report, fmt.Errorf("parse unknown debit account %s: %w", operation.id, err)
		}
		creditID, err := tigerbeetle.HexStringToUint128(operation.creditID)
		if err != nil {
			return report, fmt.Errorf("parse unknown credit account %s: %w", operation.id, err)
		}
		expected := tigerbeetle.Transfer{
			ID:              transferID,
			DebitAccountID:  debitID,
			CreditAccountID: creditID,
			Amount:          tigerbeetle.ToUint128(uint64(operation.amount)),
			Ledger:          ledgerCode,
			Code:            transferCode,
		}

		transfers, err := s.client.LookupTransfers([]tigerbeetle.Uint128{transferID})
		if err != nil {
			return report, fmt.Errorf("lookup unknown transfer %s: %w", operation.id, err)
		}
		if len(transfers) == 0 {
			report.StillUnknown++
			continue
		}
		if len(transfers) != 1 || !sameTransfer(transfers[0], expected) {
			if _, err := s.failOperation(ctx, operation, "ledger_rejected", RequestMetadata{}); err != nil && !errors.Is(err, ErrLedgerRejected) {
				return report, fmt.Errorf("mark mismatched transfer %s: %w", operation.id, err)
			}
			report.Failed++
			continue
		}
		if _, err := s.completeOperation(ctx, operation, RequestMetadata{}); err != nil {
			return report, fmt.Errorf("complete recovered operation %s: %w", operation.id, err)
		}
		report.Completed++
	}
	if err := rows.Err(); err != nil {
		return report, fmt.Errorf("read unknown ledger operations: %w", err)
	}
	return report, nil
}
