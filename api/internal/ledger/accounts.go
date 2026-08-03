package ledger

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"

	"github.com/hypernova-banking/api/internal/audit"
)

// EnsureSystemAccount creates the balancing account used for deposits and
// withdrawals. Repeating startup is safe because TigerBeetle account IDs are
// immutable and duplicate creation returns AccountExists.
func (s *Service) EnsureSystemAccount(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrLedgerUnavailable
	}
	account := tigerbeetle.Account{
		ID:     systemAccountID(s.currency),
		Ledger: ledgerCode,
		Code:   transferCode,
		Flags:  tigerbeetle.AccountFlags{History: true}.ToUint16(),
	}
	results, err := s.client.CreateAccounts([]tigerbeetle.Account{account})
	if err != nil {
		return fmt.Errorf("create system account: %w", err)
	}
	for _, result := range results {
		if result.Status != tigerbeetle.AccountCreated && result.Status != tigerbeetle.AccountExists {
			return fmt.Errorf("create system account: %s", result.Status)
		}
	}
	return nil
}

// ReconcileActiveAccounts restores the ledger identity for active PostgreSQL
// records and completes interrupted provisioning records. It never invents
// balances or transfers: recreated accounts start at zero and TigerBeetle
// remains the only source of financial truth. This is useful for local
// development after replacing a replica volume while retaining the identity
// database.
func (s *Service) ReconcileActiveAccounts(ctx context.Context) error {
	if s == nil || s.pool == nil || s.client == nil {
		return ErrLedgerUnavailable
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, tigerbeetle_account_id, status
		FROM ledger_accounts
		WHERE status IN ('active', 'provisioning')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return fmt.Errorf("list active ledger identities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var publicID, encodedID, status string
		if err := rows.Scan(&publicID, &encodedID, &status); err != nil {
			return fmt.Errorf("scan ledger identity: %w", err)
		}
		accountID, err := tigerbeetle.HexStringToUint128(encodedID)
		if err != nil {
			return fmt.Errorf("parse ledger identity: %w", err)
		}
		found, err := s.client.LookupAccounts([]tigerbeetle.Uint128{accountID})
		if err != nil {
			return fmt.Errorf("lookup ledger identity: %w", err)
		}
		if len(found) > 0 {
			if status == "provisioning" {
				if _, err := s.pool.Exec(ctx, `UPDATE ledger_accounts SET status = 'active', updated_at = NOW() WHERE id = $1 AND status = 'provisioning'`, publicID); err != nil {
					return fmt.Errorf("activate recovered ledger account: %w", err)
				}
			}
			continue
		}
		results, err := s.client.CreateAccounts([]tigerbeetle.Account{{
			ID:     accountID,
			Ledger: ledgerCode,
			Code:   transferCode,
			Flags: tigerbeetle.AccountFlags{
				DebitsMustNotExceedCredits: true,
				History:                    true,
			}.ToUint16(),
		}})
		if err != nil {
			return fmt.Errorf("recreate missing ledger identity: %w", err)
		}
		for _, result := range results {
			if result.Status != tigerbeetle.AccountCreated && result.Status != tigerbeetle.AccountExists {
				return fmt.Errorf("recreate missing ledger identity: %s", result.Status)
			}
		}
		if _, err := s.pool.Exec(ctx, `UPDATE ledger_accounts SET status = 'active', updated_at = NOW() WHERE id = $1 AND status = 'provisioning'`, publicID); err != nil {
			return fmt.Errorf("activate recreated ledger account: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read active ledger identities: %w", err)
	}
	return nil
}

// AccountView is the public account representation.
type AccountView struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Currency    string    `json:"currency"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
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
		RETURNING id, COALESCE(display_name, ''), tigerbeetle_account_id, account_type, currency, status, created_at
	`, accountID, userID, reservedTigerID, currency).Scan(&view.ID, &view.DisplayName, &persistedTigerID, &view.Type, &view.Currency, &view.Status, &view.CreatedAt)
	if err != nil {
		return AccountView{}, fmt.Errorf("reserve ledger account: %w", err)
	}
	if view.Status == "active" {
		return view, nil
	}
	parsedTigerID, err := tigerbeetle.HexStringToUint128(persistedTigerID)
	if err != nil {
		return AccountView{}, fmt.Errorf("parse reserved account id: %w", err)
	}
	account := tigerbeetle.Account{
		ID:     parsedTigerID,
		Ledger: ledgerCode,
		Code:   transferCode,
		Flags: tigerbeetle.AccountFlags{
			DebitsMustNotExceedCredits: true,
			History:                    true,
		}.ToUint16(),
	}
	results, err := s.client.CreateAccounts([]tigerbeetle.Account{account})
	if err != nil {
		return AccountView{}, fmt.Errorf("create ledger account: %w", err)
	}
	for _, result := range results {
		if result.Status != tigerbeetle.AccountCreated && result.Status != tigerbeetle.AccountExists {
			_, _ = s.pool.Exec(ctx, "UPDATE ledger_accounts SET status = 'failed', updated_at = NOW() WHERE id = $1", view.ID)
			return AccountView{}, fmt.Errorf("create ledger account: %s", result.Status)
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
		SELECT id::text, COALESCE(display_name, ''), currency, account_type, status, created_at
		FROM ledger_accounts WHERE user_id = $1 AND status = 'active' ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	defer rows.Close()
	accounts := make([]AccountView, 0)
	for rows.Next() {
		var account AccountView
		if err := rows.Scan(&account.ID, &account.DisplayName, &account.Currency, &account.Type, &account.Status, &account.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ledger accounts: %w", err)
	}
	return accounts, nil
}

// EnsureInitialAccount repairs a partially completed registration without
// creating duplicates when an active account already exists.
func (s *Service) EnsureInitialAccount(ctx context.Context, userID uuid.UUID, metadata ...RequestMetadata) (AccountView, error) {
	accounts, err := s.ListAccounts(ctx, userID)
	if err != nil {
		return AccountView{}, err
	}
	if len(accounts) > 0 {
		return accounts[0], nil
	}
	return s.CreateAccount(ctx, userID, defaultCurrency, metadata...)
}

// CloseAccount marks an owned account as closed when its available and
// pending TigerBeetle amounts are all zero. This preserves immutable ledger
// history while preventing further operations through active-account guards.
func (s *Service) CloseAccount(ctx context.Context, userID uuid.UUID, publicID string, metadata ...RequestMetadata) error {
	if s == nil || s.pool == nil || s.client == nil || userID == uuid.Nil {
		return ErrInvalidInput
	}
	account, err := s.accountForUser(ctx, userID, publicID)
	if err != nil {
		return err
	}
	tbID, err := tigerbeetle.HexStringToUint128(account.tigerID)
	if err != nil {
		return ErrLedgerRejected
	}
	accounts, err := s.client.LookupAccounts([]tigerbeetle.Uint128{tbID})
	if err != nil {
		return fmt.Errorf("lookup account before close: %w", err)
	}
	if len(accounts) != 1 || accounts[0].ID != tbID {
		return ErrNotFound
	}
	ledgerAccount := accounts[0]
	balance := new(big.Int).Sub(ledgerAccount.CreditsPosted.BigInt(), ledgerAccount.DebitsPosted.BigInt())
	if balance.Sign() != 0 || ledgerAccount.CreditsPending.BigInt().Sign() != 0 || ledgerAccount.DebitsPending.BigInt().Sign() != 0 {
		return ErrAccountNotEmpty
	}
	accountID, err := uuid.Parse(account.publicID)
	if err != nil {
		return ErrInvalidInput
	}
	if tag, err := s.pool.Exec(ctx, `UPDATE ledger_accounts SET status = 'closed', updated_at = NOW() WHERE id = $1 AND user_id = $2 AND status = 'active'`, accountID, userID); err != nil {
		return fmt.Errorf("close ledger account: %w", err)
	} else if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	requestMetadata := RequestMetadata{}
	if len(metadata) > 0 {
		requestMetadata = metadata[0]
	}
	if err := audit.Record(ctx, s.pool, &userID, "account_closed", map[string]any{"account_id": publicID}, requestMetadata.IPAddress, requestMetadata.UserAgent); err != nil {
		return fmt.Errorf("audit account closure: %w", err)
	}
	return nil
}

// Account returns one account after checking ownership.
func (s *Service) Account(ctx context.Context, userID uuid.UUID, publicID string) (AccountView, error) {
	account, err := s.accountForUser(ctx, userID, publicID)
	if err != nil {
		return AccountView{}, err
	}
	var view AccountView
	err = s.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(display_name, ''), currency, account_type, status, created_at
		FROM ledger_accounts WHERE id = $1
	`, account.publicID).Scan(&view.ID, &view.DisplayName, &view.Currency, &view.Type, &view.Status, &view.CreatedAt)
	if err != nil {
		return AccountView{}, fmt.Errorf("load ledger account: %w", err)
	}
	return view, nil
}

// RenameAccount updates only the user-facing label after re-checking ownership.
func (s *Service) RenameAccount(ctx context.Context, userID uuid.UUID, publicID, displayName string, metadata ...RequestMetadata) (AccountView, error) {
	if s == nil || s.pool == nil || userID == uuid.Nil {
		return AccountView{}, ErrInvalidInput
	}
	account, err := s.accountForUser(ctx, userID, publicID)
	if err != nil {
		return AccountView{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if utf8.RuneCountInString(displayName) < 2 || utf8.RuneCountInString(displayName) > 48 {
		return AccountView{}, ErrInvalidInput
	}
	accountID, err := uuid.Parse(account.publicID)
	if err != nil {
		return AccountView{}, ErrInvalidInput
	}
	var view AccountView
	err = s.pool.QueryRow(ctx, `
		UPDATE ledger_accounts SET display_name = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3
		RETURNING id::text, COALESCE(display_name, ''), currency, account_type, status, created_at
	`, displayName, accountID, userID).Scan(&view.ID, &view.DisplayName, &view.Currency, &view.Type, &view.Status, &view.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountView{}, ErrNotFound
		}
		return AccountView{}, fmt.Errorf("rename ledger account: %w", err)
	}
	requestMetadata := RequestMetadata{}
	if len(metadata) > 0 {
		requestMetadata = metadata[0]
	}
	if err := audit.Record(ctx, s.pool, &userID, "account_renamed", map[string]any{"account_id": publicID}, requestMetadata.IPAddress, requestMetadata.UserAgent); err != nil {
		return AccountView{}, fmt.Errorf("audit account rename: %w", err)
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
	tbID, err := tigerbeetle.HexStringToUint128(account.tigerID)
	if err != nil {
		return BalanceView{}, ErrLedgerRejected
	}
	accounts, err := s.client.LookupAccounts([]tigerbeetle.Uint128{tbID})
	if err != nil {
		return BalanceView{}, fmt.Errorf("lookup balance: %w", err)
	}
	if len(accounts) != 1 || accounts[0].ID != tbID {
		return BalanceView{}, ErrNotFound
	}
	value := accounts[0]
	credits := value.CreditsPosted.BigInt()
	debits := value.DebitsPosted.BigInt()
	balance := new(big.Int).Sub(credits, debits)
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
	ownerID  uuid.UUID
	currency string
	status   string
}

func (s *Service) accountForUser(ctx context.Context, userID uuid.UUID, publicID string) (accountRecord, error) {
	account, err := s.accountByPublicID(ctx, publicID)
	if err != nil {
		return accountRecord{}, err
	}
	if account.ownerID != userID || account.status != "active" {
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
		SELECT id::text, user_id, tigerbeetle_account_id, currency, status
		FROM ledger_accounts WHERE id = $1
	`, id).Scan(&account.publicID, &account.ownerID, &account.tigerID, &account.currency, &account.status)
	if errors.Is(err, pgx.ErrNoRows) {
		return accountRecord{}, ErrNotFound
	}
	if err != nil {
		return accountRecord{}, fmt.Errorf("find ledger account: %w", err)
	}
	return account, nil
}
