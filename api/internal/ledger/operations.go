package ledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
)

// OperationView is the stable response returned by all posted movements.
type OperationView struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	TransferID string    `json:"transfer_id"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"created_at"`
}

// Deposit credits a user account from the controlled system account.
func (s *Service) Deposit(ctx context.Context, userID uuid.UUID, accountID, amount, key string, metadata RequestMetadata) (OperationView, error) {
	if s == nil || !s.config.AllowDemoDeposits {
		return OperationView{}, ErrForbidden
	}
	account, err := s.accountForUser(ctx, userID, accountID)
	if err != nil {
		return OperationView{}, err
	}
	return s.execute(ctx, userID, "deposit", account, systemAccountID(s.currency), amount, key, metadata, "")
}

// Withdraw debits a user account to the controlled system account. The
// TigerBeetle account invariant rejects withdrawals above available funds.
func (s *Service) Withdraw(ctx context.Context, userID uuid.UUID, accountID, amount, key string, metadata RequestMetadata) (OperationView, error) {
	account, err := s.accountForUser(ctx, userID, accountID)
	if err != nil {
		return OperationView{}, err
	}
	return s.execute(ctx, userID, "withdrawal", account, systemAccountID(s.currency), amount, key, metadata, "")
}

// Transfer preserves the original API behavior: transfers are between the
// authenticated user's own accounts.
func (s *Service) Transfer(ctx context.Context, userID uuid.UUID, fromID, toID, amount, key string, metadata RequestMetadata) (OperationView, error) {
	return s.TransferWithScope(ctx, userID, fromID, toID, amount, key, string(TransferScopeOwn), metadata)
}

// TransferWithScope validates whether the destination is an own account or an
// active account belonging to another user before entering the idempotent
// TigerBeetle execution path.
func (s *Service) TransferWithScope(ctx context.Context, userID uuid.UUID, fromID, toID, amount, key, requestedScope string, metadata RequestMetadata) (OperationView, error) {
	scope, from, to, err := s.transferAccounts(ctx, userID, fromID, toID, requestedScope)
	if err != nil {
		return OperationView{}, err
	}
	return s.execute(ctx, userID, "transfer", from, mustTigerID(to.tigerID), amount, key, metadata, string(scope), to)
}

// ValidateTransferTarget checks the destination before a caller collects a
// confirmation PIN. It does not reserve idempotency state or touch the ledger.
func (s *Service) ValidateTransferTarget(ctx context.Context, userID uuid.UUID, fromID, toID, requestedScope string) error {
	_, _, _, err := s.transferAccounts(ctx, userID, fromID, toID, requestedScope)
	return err
}

func (s *Service) transferAccounts(ctx context.Context, userID uuid.UUID, fromID, toID, requestedScope string) (TransferScope, accountRecord, accountRecord, error) {
	scope, err := NormalizeTransferScope(requestedScope)
	if err != nil {
		return "", accountRecord{}, accountRecord{}, err
	}
	from, err := s.accountForUser(ctx, userID, fromID)
	if err != nil {
		return "", accountRecord{}, accountRecord{}, err
	}
	to, err := s.accountByPublicID(ctx, toID)
	if err != nil {
		return "", accountRecord{}, accountRecord{}, err
	}
	if from.tigerID == to.tigerID || from.currency != to.currency {
		return "", accountRecord{}, accountRecord{}, ErrInvalidInput
	}
	if to.status != "active" {
		return "", accountRecord{}, accountRecord{}, ErrNotFound
	}
	if scope == TransferScopeOwn && to.ownerID != userID {
		return "", accountRecord{}, accountRecord{}, ErrNotFound
	}
	if scope == TransferScopeExternal && to.ownerID == userID {
		return "", accountRecord{}, accountRecord{}, ErrInvalidInput
	}
	return scope, from, to, nil
}

// HistoryView contains immutable TigerBeetle transfer facts plus the local
// operation classification when it is available.
type HistoryView struct {
	TransferID string    `json:"transfer_id"`
	Type       string    `json:"type"`
	Direction  string    `json:"direction"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"created_at"`
}

// History returns the most recent account transfers from TigerBeetle.
func (s *Service) History(ctx context.Context, userID uuid.UUID, publicAccountID string, limit uint32, cursor string) ([]HistoryView, error) {
	account, err := s.accountForUser(ctx, userID, publicAccountID)
	if err != nil {
		return nil, err
	}
	if limit == 0 || limit > maxHistory {
		limit = 50
	}
	tbID := mustTigerID(account.tigerID)
	filter := tigerbeetle.AccountFilter{
		AccountID: tbID,
		// Read one sentinel row so the HTTP adapter can expose an accurate
		// has_more value instead of guessing when a page is exactly full.
		Limit: limit + 1,
		Flags: tigerbeetle.AccountFilterFlags{
			Debits:   true,
			Credits:  true,
			Reversed: true,
		}.ToUint32(),
	}
	if strings.TrimSpace(cursor) != "" {
		value, parseErr := strconv.ParseUint(strings.TrimSpace(cursor), 10, 64)
		if parseErr != nil || value == 0 {
			return nil, ErrInvalidInput
		}
		filter.TimestampMax = value - 1
	}
	transfers, err := s.client.GetAccountTransfers(filter)
	if err != nil {
		return nil, fmt.Errorf("lookup history: %w", err)
	}
	metadata := s.transferMetadata(ctx, transfers)
	result := make([]HistoryView, 0, len(transfers))
	for _, transfer := range transfers {
		transferID := transfer.ID.String()
		item := HistoryView{
			TransferID: transferID,
			Type:       "transfer",
			Direction:  "debit",
			Amount:     uint128String(transfer.Amount),
			Currency:   account.currency,
			CreatedAt:  time.Unix(0, int64(transfer.Timestamp)).UTC(),
		}
		if transfer.CreditAccountID == tbID {
			item.Direction = "credit"
		}
		if value, ok := metadata[transferID]; ok {
			item.Type = value.operationType
		}
		result = append(result, item)
	}
	return result, nil
}

// execute is the single financial execution path for deposits, withdrawals
// and transfers. It validates the request, reserves idempotency state, calls
// TigerBeetle and delegates durable outcome handling to operation persistence.
func (s *Service) execute(ctx context.Context, userID uuid.UUID, operationType string, from accountRecord, to tigerbeetle.Uint128, amount, key string, metadata RequestMetadata, intentScope string, destinations ...accountRecord) (OperationView, error) {
	minor, err := parseMinorAmount(amount)
	if err != nil || strings.TrimSpace(key) == "" || len(key) > 128 {
		return OperationView{}, ErrInvalidInput
	}
	debitID := mustTigerID(from.tigerID)
	creditID := to
	if operationType == "deposit" {
		debitID, creditID = to, debitID
	}
	if operationType == "transfer" && len(destinations) == 0 {
		return OperationView{}, ErrInvalidInput
	}
	requestHash := hashRequestWithScope(operationType, debitID, creditID, minor, from.currency, intentScope)
	operation, replay, err := s.startOperation(ctx, userID, key, requestHash, operationType, debitID.String(), creditID.String(), minor, from.currency)
	if err != nil {
		return OperationView{}, err
	}
	if replay {
		return operation.result, operation.replayError()
	}
	if operation.status == "processing" && operation.createdAt.Before(time.Now().UTC().Add(-30*time.Second)) {
		// A process can die after reserving the operation. Treating an old
		// reservation as unknown allows the original TigerBeetle ID to be
		// reconciled safely on the next retry.
		if _, err := s.pool.Exec(ctx, "UPDATE ledger_operations SET status = 'unknown', updated_at = NOW() WHERE id = $1 AND status = 'processing'", operation.id); err != nil {
			return OperationView{}, fmt.Errorf("mark stale operation: %w", err)
		}
		operation.status = "unknown"
	}
	if s.client == nil {
		return s.markOperationUnknown(ctx, operation, metadata)
	}
	transfer := tigerbeetle.Transfer{
		ID:              mustTigerID(operation.transferID),
		DebitAccountID:  debitID,
		CreditAccountID: creditID,
		Amount:          tigerbeetle.ToUint128(uint64(minor)),
		Ledger:          ledgerCode,
		Code:            transferCode,
	}
	results, err := s.client.CreateTransfers([]tigerbeetle.Transfer{transfer})
	if err != nil {
		return s.markOperationUnknown(ctx, operation, metadata)
	}
	for _, result := range results {
		switch result.Status {
		case tigerbeetle.TransferCreated:
			return s.completeOperation(ctx, operation, metadata)
		case tigerbeetle.TransferExists:
			return s.reconcileExistingTransfer(ctx, operation, transfer, metadata)
		case tigerbeetle.TransferExceedsCredits, tigerbeetle.TransferExceedsDebits:
			return s.failOperation(ctx, operation, "insufficient_funds", metadata)
		default:
			return s.failOperation(ctx, operation, "ledger_rejected", metadata)
		}
	}
	return s.completeOperation(ctx, operation, metadata)
}

type transferIndex struct{ operationType string }

func (s *Service) transferMetadata(ctx context.Context, transfers []tigerbeetle.Transfer) map[string]transferIndex {
	ids := make([]string, 0, len(transfers))
	for _, transfer := range transfers {
		ids = append(ids, transfer.ID.String())
	}
	result := make(map[string]transferIndex, len(ids))
	if len(ids) == 0 {
		return result
	}
	rows, err := s.pool.Query(ctx, "SELECT tigerbeetle_transfer_id, operation_type FROM ledger_transfers WHERE tigerbeetle_transfer_id = ANY($1::text[])", ids)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id, operationType string
		if rows.Scan(&id, &operationType) == nil {
			result[id] = transferIndex{operationType: operationType}
		}
	}
	return result
}
