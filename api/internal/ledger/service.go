// Package ledger coordinates the financial API with TigerBeetle.
//
// PostgreSQL stores ownership, replay controls and an audit-friendly index;
// TigerBeetle remains the only source of truth for balances and transfers.
package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"

	"github.com/hypernova-banking/api/internal/audit"
)

const (
	defaultCurrency = "HNL"
	ledgerCode      = uint32(1)
	transferCode    = uint16(1)
	maxHistory      = uint32(100)
)

var (
	ErrInvalidInput        = errors.New("invalid ledger input")
	ErrNotFound            = errors.New("ledger resource not found")
	ErrConflict            = errors.New("ledger conflict")
	ErrForbidden           = errors.New("ledger operation forbidden")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrLedgerUnavailable   = errors.New("ledger unavailable")
	ErrLedgerRejected      = errors.New("ledger rejected operation")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
)

// Client is the small TigerBeetle surface used by this service. Keeping it
// local makes domain tests independent from a running ledger process.
type Client interface {
	CreateAccounts([]types.Account) ([]types.AccountEventResult, error)
	CreateTransfers([]types.Transfer) ([]types.TransferEventResult, error)
	LookupAccounts([]types.Uint128) ([]types.Account, error)
	LookupTransfers([]types.Uint128) ([]types.Transfer, error)
	GetAccountTransfers(types.AccountFilter) ([]types.Transfer, error)
	Nop() error
}

// RequestMetadata contains only non-secret request information for audit.
type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

// Service exposes account and transfer use cases.
type Service struct {
	pool     *pgxpool.Pool
	client   Client
	currency string
	config   Config
}

// Config controls deliberately explicit development-only capabilities.
// Deposits are disabled unless the deployment opts into the demo clearing
// account; production integrations should replace it with a trusted funding
// provider or an operator-authorized workflow.
type Config struct {
	AllowDemoDeposits bool
}

// NewService creates a ledger service for the supported HNL account model.
func NewService(pool *pgxpool.Pool, client Client, configs ...Config) *Service {
	config := Config{}
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Service{pool: pool, client: client, currency: defaultCurrency, config: config}
}

// Ping lets readiness verify that the TigerBeetle client can reach its node.
func (s *Service) Ping(context.Context) error {
	if s == nil || s.client == nil {
		return ErrLedgerUnavailable
	}
	if err := s.client.Nop(); err != nil {
		return fmt.Errorf("ping tigerbeetle: %w", err)
	}
	return nil
}

// EnsureSystemAccount creates the balancing account used for deposits and
// withdrawals. Repeating startup is safe because TigerBeetle account IDs are
// immutable and duplicate creation returns AccountExists.
func (s *Service) EnsureSystemAccount(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrLedgerUnavailable
	}
	account := types.Account{
		ID:     systemAccountID(s.currency),
		Ledger: ledgerCode,
		Code:   transferCode,
		Flags:  types.AccountFlags{History: true}.ToUint16(),
	}
	results, err := s.client.CreateAccounts([]types.Account{account})
	if err != nil {
		return fmt.Errorf("create system account: %w", err)
	}
	for _, result := range results {
		if result.Result != types.AccountOK && result.Result != types.AccountExists {
			return fmt.Errorf("create system account: %s", result.Result)
		}
	}
	return nil
}

// AccountView is the public account representation.
type AccountView struct {
	ID        string    `json:"id"`
	Currency  string    `json:"currency"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAccount provisions one user-owned checking account.
func (s *Service) CreateAccount(ctx context.Context, userID uuid.UUID, currency string, metadata ...RequestMetadata) (AccountView, error) {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return AccountView{}, ErrInvalidInput
	}
	currency, err := normalizeCurrency(currency)
	if err != nil {
		return AccountView{}, err
	}
	accountID := uuid.New()
	reservedTigerID := uuidToTigerID(accountID).String()
	var view AccountView
	var persistedTigerID string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO ledger_accounts (id, user_id, tigerbeetle_account_id, account_type, currency)
		VALUES ($1, $2, $3, 'checking', $4)
		ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = NOW()
		RETURNING id, tigerbeetle_account_id, account_type, currency, status, created_at
	`, accountID, userID, reservedTigerID, currency).Scan(&view.ID, &persistedTigerID, &view.Type, &view.Currency, &view.Status, &view.CreatedAt)
	if err != nil {
		return AccountView{}, fmt.Errorf("reserve ledger account: %w", err)
	}
	if view.Status == "active" {
		return view, nil
	}
	parsedTigerID, err := types.HexStringToUint128(persistedTigerID)
	if err != nil {
		return AccountView{}, fmt.Errorf("parse reserved account id: %w", err)
	}
	account := types.Account{
		ID:     parsedTigerID,
		Ledger: ledgerCode,
		Code:   transferCode,
		Flags: types.AccountFlags{
			DebitsMustNotExceedCredits: true,
			History:                    true,
		}.ToUint16(),
	}
	results, err := s.client.CreateAccounts([]types.Account{account})
	if err != nil {
		return AccountView{}, fmt.Errorf("create ledger account: %w", err)
	}
	for _, result := range results {
		if result.Result != types.AccountOK && result.Result != types.AccountExists {
			_, _ = s.pool.Exec(ctx, "UPDATE ledger_accounts SET status = 'failed', updated_at = NOW() WHERE id = $1", view.ID)
			return AccountView{}, fmt.Errorf("create ledger account: %s", result.Result)
		}
	}
	if _, err := s.pool.Exec(ctx, "UPDATE ledger_accounts SET status = 'active', updated_at = NOW() WHERE id = $1", view.ID); err != nil {
		return AccountView{}, fmt.Errorf("activate ledger account: %w", err)
	}
	requestMetadata := RequestMetadata{}
	if len(metadata) > 0 {
		requestMetadata = metadata[0]
	}
	if err := audit.Record(ctx, s.pool, &userID, "account_created", map[string]any{"account_id": view.ID, "currency": view.Currency}, requestMetadata.IPAddress, requestMetadata.UserAgent); err != nil {
		return AccountView{}, fmt.Errorf("audit account creation: %w", err)
	}
	view.Status = "active"
	return view, nil
}

// ListAccounts returns only accounts owned by the authenticated user.
func (s *Service) ListAccounts(ctx context.Context, userID uuid.UUID) ([]AccountView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, currency, account_type, status, created_at
		FROM ledger_accounts WHERE user_id = $1 ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	defer rows.Close()
	accounts := make([]AccountView, 0)
	for rows.Next() {
		var account AccountView
		if err := rows.Scan(&account.ID, &account.Currency, &account.Type, &account.Status, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ledger accounts: %w", err)
	}
	return accounts, nil
}

// Account returns one account after checking ownership.
func (s *Service) Account(ctx context.Context, userID uuid.UUID, publicID string) (AccountView, error) {
	account, err := s.accountForUser(ctx, userID, publicID)
	if err != nil {
		return AccountView{}, err
	}
	var view AccountView
	err = s.pool.QueryRow(ctx, `
		SELECT id::text, currency, account_type, status, created_at
		FROM ledger_accounts WHERE id = $1
	`, account.publicID).Scan(&view.ID, &view.Currency, &view.Type, &view.Status, &view.CreatedAt)
	if err != nil {
		return AccountView{}, fmt.Errorf("load ledger account: %w", err)
	}
	return view, nil
}

// BalanceView reports integer minor units as strings so JSON clients cannot
// silently round financial values through floating-point numbers.
type BalanceView struct {
	AccountID      string `json:"account_id"`
	Currency       string `json:"currency"`
	Balance        string `json:"balance"`
	Available      string `json:"available_balance"`
	CreditsPosted  string `json:"credits_posted"`
	DebitsPosted   string `json:"debits_posted"`
	CreditsPending string `json:"credits_pending"`
	DebitsPending  string `json:"debits_pending"`
}

// Balance reads the balance from TigerBeetle and never from a PostgreSQL
// counter or a cached projection.
func (s *Service) Balance(ctx context.Context, userID uuid.UUID, publicAccountID string) (BalanceView, error) {
	account, err := s.accountForUser(ctx, userID, publicAccountID)
	if err != nil {
		return BalanceView{}, err
	}
	tbID, err := types.HexStringToUint128(account.tigerID)
	if err != nil {
		return BalanceView{}, ErrLedgerRejected
	}
	accounts, err := s.client.LookupAccounts([]types.Uint128{tbID})
	if err != nil {
		return BalanceView{}, fmt.Errorf("lookup balance: %w", err)
	}
	if len(accounts) != 1 || accounts[0].ID != tbID {
		return BalanceView{}, ErrNotFound
	}
	value := accounts[0]
	credits := value.CreditsPosted.BigInt()
	debits := value.DebitsPosted.BigInt()
	balance := new(big.Int).Sub(&credits, &debits)
	if balance.Sign() < 0 {
		balance.SetInt64(0)
	}
	return BalanceView{
		AccountID:      publicAccountID,
		Currency:       account.currency,
		Balance:        balance.String(),
		Available:      balance.String(),
		CreditsPosted:  uint128String(value.CreditsPosted),
		DebitsPosted:   uint128String(value.DebitsPosted),
		CreditsPending: uint128String(value.CreditsPending),
		DebitsPending:  uint128String(value.DebitsPending),
	}, nil
}

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
		Limit:     limit,
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

type accountRecord struct {
	publicID string
	tigerID  string
	currency string
	status   string
}

func (s *Service) accountForUser(ctx context.Context, userID uuid.UUID, publicID string) (accountRecord, error) {
	account, err := s.accountByPublicID(ctx, publicID)
	if err != nil {
		return accountRecord{}, err
	}
	var owner uuid.UUID
	err = s.pool.QueryRow(ctx, "SELECT user_id FROM ledger_accounts WHERE id = $1 AND status = 'active'", publicID).Scan(&owner)
	if err != nil || owner != userID {
		return accountRecord{}, ErrNotFound
	}
	return account, nil
}

func (s *Service) accountByPublicID(ctx context.Context, publicID string) (accountRecord, error) {
	id, err := uuid.Parse(publicID)
	if err != nil {
		return accountRecord{}, ErrInvalidInput
	}
	var account accountRecord
	err = s.pool.QueryRow(ctx, `
		SELECT id::text, tigerbeetle_account_id, currency, status
		FROM ledger_accounts WHERE id = $1
	`, id).Scan(&account.publicID, &account.tigerID, &account.currency, &account.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountRecord{}, ErrNotFound
	}
	if err != nil {
		return accountRecord{}, fmt.Errorf("find ledger account: %w", err)
	}
	return account, nil
}

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
	operationID := uuid.New()
	transferID := types.ID().String()
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

func (s *Service) reconcileExistingTransfer(ctx context.Context, operation operationRecord, expected types.Transfer, metadata RequestMetadata) (OperationView, error) {
	existing, err := s.client.LookupTransfers([]types.Uint128{expected.ID})
	if err != nil || len(existing) != 1 {
		return s.markOperationUnknown(ctx, operation, metadata)
	}
	if !sameTransfer(existing[0], expected) {
		return s.failOperation(ctx, operation, "ledger_rejected", metadata)
	}
	return s.completeOperation(ctx, operation, metadata)
}

func sameTransfer(actual, expected types.Transfer) bool {
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

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = defaultCurrency
	}
	if value != defaultCurrency {
		return "", ErrInvalidInput
	}
	return value, nil
}

func parseMinorAmount(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, ErrInvalidInput
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, ErrInvalidInput
		}
	}
	parsed, err := strconv.ParseUint(value, 10, 63)
	if err != nil || parsed == 0 {
		return 0, ErrInvalidInput
	}
	return int64(parsed), nil
}

func hashRequest(operationType string, debit, credit types.Uint128, amount int64, currency string) []byte {
	value := fmt.Sprintf("%s|%s|%s|%d|%s", operationType, debit.String(), credit.String(), amount, currency)
	hash := sha256.Sum256([]byte(value))
	return hash[:]
}

func uint128String(value types.Uint128) string {
	parsed := value.BigInt()
	return parsed.String()
}

func uuidToTigerID(id uuid.UUID) types.Uint128 {
	var bytesValue [16]byte
	copy(bytesValue[:], id[:])
	return types.BytesToUint128(bytesValue)
}

func systemAccountID(currency string) types.Uint128 {
	// A fixed namespace keeps development system accounts stable without
	// overlapping normal UUID-v4 account identifiers in practice.
	if currency == defaultCurrency {
		return types.ToUint128(0x1000000000000001)
	}
	return types.ToUint128(0x1000000000000002)
}

func mustTigerID(value string) types.Uint128 {
	parsed, err := types.HexStringToUint128(value)
	if err != nil {
		panic("invalid persisted TigerBeetle identifier: " + hex.EncodeToString([]byte(value)))
	}
	return parsed
}
