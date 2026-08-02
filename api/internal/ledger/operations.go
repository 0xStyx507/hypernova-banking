package ledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
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
	return s.execute(ctx, userID, "deposit", account, systemAccountID(s.currency), amount, key, metadata)
}

// Withdraw debits a user account to the controlled system account. The
// TigerBeetle account invariant rejects withdrawals above available funds.
func (s *Service) Withdraw(ctx context.Context, userID uuid.UUID, accountID, amount, key string, metadata RequestMetadata) (OperationView, error) {
	account, err := s.accountForUser(ctx, userID, accountID)
	if err != nil {
		return OperationView{}, err
	}
	return s.execute(ctx, userID, "withdrawal", account, systemAccountID(s.currency), amount, key, metadata)
}

// Transfer moves funds from an owned account to another active account.
func (s *Service) Transfer(ctx context.Context, userID uuid.UUID, fromID, toID, amount, key string, metadata RequestMetadata) (OperationView, error) {
	from, err := s.accountForUser(ctx, userID, fromID)
	if err != nil {
		return OperationView{}, err
	}
	to, err := s.accountByPublicID(ctx, toID)
	if err != nil {
		return OperationView{}, err
	}
	if from.tigerID == to.tigerID || to.status != "active" {
		return OperationView{}, ErrInvalidInput
	}
	return s.execute(ctx, userID, "transfer", from, mustTigerID(to.tigerID), amount, key, metadata, to)
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
	filter := types.AccountFilter{
		AccountID: tbID,
		// Read one sentinel row so the HTTP adapter can expose an accurate
		// has_more value instead of guessing when a page is exactly full.
		Limit: limit + 1,
		Flags: types.AccountFilterFlags{
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
func (s *Service) execute(ctx context.Context, userID uuid.UUID, operationType string, from accountRecord, to types.Uint128, amount, key string, metadata RequestMetadata, destinations ...accountRecord) (OperationView, error) {
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
	requestHash := hashRequest(operationType, debitID, creditID, minor, from.currency)
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
	transfer := types.Transfer{
		ID:              mustTigerID(operation.transferID),
		DebitAccountID:  debitID,
		CreditAccountID: creditID,
		Amount:          types.ToUint128(uint64(minor)),
		Ledger:          ledgerCode,
		Code:            transferCode,
	}
	results, err := s.client.CreateTransfers([]types.Transfer{transfer})
	if err != nil {
		return s.markOperationUnknown(ctx, operation, metadata)
	}
	for _, result := range results {
		switch result.Result {
		case types.TransferOK:
			return s.completeOperation(ctx, operation, metadata)
		case types.TransferExists:
			return s.reconcileExistingTransfer(ctx, operation, transfer, metadata)
		case types.TransferExceedsCredits, types.TransferExceedsDebits:
			return s.failOperation(ctx, operation, "insufficient_funds", metadata)
		default:
			return s.failOperation(ctx, operation, "ledger_rejected", metadata)
		}
	}
	return s.completeOperation(ctx, operation, metadata)
}

type transferIndex struct{ operationType string }

func (s *Service) transferMetadata(ctx context.Context, transfers []types.Transfer) map[string]transferIndex {
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
