package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"

	"github.com/hypernova-banking/api/internal/audit"
)

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
