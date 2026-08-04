// Package seed imports the development fixture's identity data.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hypernova-banking/api/internal/audit"
	"github.com/hypernova-banking/api/internal/auth"
)

// fixture contains the complete input shape; this command persists identities
// while accounts and transactions are created through the financial API.
type fixture struct {
	Users        []userRecord        `json:"users"`
	Accounts     []accountRecord     `json:"accounts"`
	Transactions []transactionRecord `json:"transactions"`
}

type userRecord struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FullName  string `json:"full_name"`
	CreatedAt string `json:"created_at"`
}

type accountRecord struct {
	AccountNumber  string      `json:"account_number"`
	UserID         string      `json:"user_id"`
	InitialBalance json.Number `json:"initial_balance"`
	Currency       string      `json:"currency"`
	AccountType    string      `json:"account_type"`
}

type transactionRecord struct {
	FromAccount string      `json:"from_account"`
	ToAccount   string      `json:"to_account"`
	Amount      json.Number `json:"amount"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Timestamp   string      `json:"timestamp"`
	Status      string      `json:"status"`
}

// Report summarizes work without printing credentials or fixture contents.
type Report struct {
	UsersProcessed       int
	AccountsImported     int
	TransactionsImported int
}

// DuplicateReport is a safe reconciliation report. It identifies source row
// positions and comparison results, but never writes emails, passwords, or
// account numbers to disk.
type DuplicateReport struct {
	DuplicateGroups  int                 `json:"duplicate_groups"`
	DuplicateRecords int                 `json:"duplicate_records"`
	Conflicts        []DuplicateConflict `json:"conflicts"`
}

// DuplicateConflict describes one normalized-email collision without exposing
// the colliding email itself.
type DuplicateConflict struct {
	Group                   int   `json:"group"`
	RecordIndexes           []int `json:"record_indexes"`
	SameFullName            bool  `json:"same_full_name"`
	SamePasswordField       bool  `json:"same_password_field"`
	SameCreatedAt           bool  `json:"same_created_at"`
	SameProfileFields       bool  `json:"same_profile_fields"`
	RecordsWithAccounts     int   `json:"records_with_accounts"`
	RecordsWithTransactions int   `json:"records_with_transactions"`
}

// Run validates and imports users in one PostgreSQL transaction. Re-running
// it updates safe profile fields but never replaces an existing password hash.
func Run(ctx context.Context, pool *pgxpool.Pool, filePath string, bcryptCost int) (Report, error) {
	input, err := loadFixture(filePath)
	if err != nil {
		return Report{}, err
	}
	if err := validateUniqueEmails(input.Users); err != nil {
		return Report{}, err
	}
	accounts, transactions, err := prepareFinancialFixture(input)
	if err != nil {
		return Report{}, err
	}
	prepared := make([]preparedUser, 0, len(input.Users))
	for _, record := range input.Users {
		user, err := prepareUser(record, bcryptCost)
		if err != nil {
			return Report{}, err
		}
		prepared = append(prepared, user)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("begin seed: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, user := range prepared {
		_, err := tx.Exec(ctx, `
			INSERT INTO users (id, email, password_hash, full_name, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, full_name = EXCLUDED.full_name, updated_at = NOW()
		`, user.ID, user.Email, user.PasswordHash, user.FullName, user.CreatedAt)
		if err != nil {
			return Report{}, fmt.Errorf("insert seed user %s: %w", user.ID, err)
		}
	}
	for _, account := range accounts {
		_, err := tx.Exec(ctx, `
			INSERT INTO fixture_accounts (account_number, user_id, initial_balance_minor, currency, account_type)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (account_number) DO UPDATE SET user_id = EXCLUDED.user_id,
				initial_balance_minor = EXCLUDED.initial_balance_minor, currency = EXCLUDED.currency,
				account_type = EXCLUDED.account_type, imported_at = NOW()
		`, account.AccountNumber, account.UserID, account.InitialBalanceMinor, account.Currency, account.AccountType)
		if err != nil {
			return Report{}, fmt.Errorf("insert fixture account %s: %w", account.AccountNumber, err)
		}
	}
	for _, transaction := range transactions {
		_, err := tx.Exec(ctx, `
			INSERT INTO fixture_transactions (source_key, from_account, to_account, amount_minor, operation_type, description, occurred_at, status, currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (source_key) DO UPDATE SET from_account = EXCLUDED.from_account,
				to_account = EXCLUDED.to_account, amount_minor = EXCLUDED.amount_minor,
				operation_type = EXCLUDED.operation_type, description = EXCLUDED.description,
				occurred_at = EXCLUDED.occurred_at, status = EXCLUDED.status, currency = EXCLUDED.currency,
				imported_at = NOW()
		`, transaction.SourceKey, transaction.FromAccount, transaction.ToAccount, transaction.AmountMinor, transaction.OperationType, transaction.Description, transaction.OccurredAt, transaction.Status, transaction.Currency)
		if err != nil {
			return Report{}, fmt.Errorf("insert fixture transaction %s: %w", transaction.SourceKey, err)
		}
	}
	if err := audit.Record(ctx, tx, nil, "seed_users", map[string]any{"users_processed": len(prepared)}, "", ""); err != nil {
		return Report{}, fmt.Errorf("audit seed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit seed: %w", err)
	}
	return Report{UsersProcessed: len(prepared), AccountsImported: len(accounts), TransactionsImported: len(transactions)}, nil
}

type preparedAccount struct {
	AccountNumber       string
	UserID              uuid.UUID
	InitialBalanceMinor int64
	Currency            string
	AccountType         string
}

type preparedTransaction struct {
	SourceKey     string
	FromAccount   string
	ToAccount     string
	AmountMinor   int64
	OperationType string
	Description   string
	OccurredAt    time.Time
	Status        string
	Currency      string
}

func prepareFinancialFixture(input fixture) ([]preparedAccount, []preparedTransaction, error) {
	users := make(map[string]struct{}, len(input.Users))
	for _, user := range input.Users {
		users[user.ID] = struct{}{}
	}
	accounts := make([]preparedAccount, 0, len(input.Accounts))
	accountNumbers := make(map[string]struct{}, len(input.Accounts))
	for _, record := range input.Accounts {
		if strings.TrimSpace(record.AccountNumber) == "" {
			return nil, nil, fmt.Errorf("fixture account number is required")
		}
		if _, exists := accountNumbers[record.AccountNumber]; exists {
			return nil, nil, fmt.Errorf("fixture contains duplicate account %s", record.AccountNumber)
		}
		if _, exists := users[record.UserID]; !exists {
			return nil, nil, fmt.Errorf("fixture account %s references unknown user", record.AccountNumber)
		}
		currency := strings.ToUpper(strings.TrimSpace(record.Currency))
		if currency == "" {
			currency = "USD"
		}
		if currency != "USD" || (record.AccountType != "checking" && record.AccountType != "savings") {
			return nil, nil, fmt.Errorf("fixture account %s has unsupported currency or type", record.AccountNumber)
		}
		balance, err := decimalToMinor(record.InitialBalance)
		if err != nil || balance < 0 {
			return nil, nil, fmt.Errorf("fixture account %s has invalid initial balance", record.AccountNumber)
		}
		userID, err := uuid.Parse(record.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("fixture account %s has invalid user id", record.AccountNumber)
		}
		accounts = append(accounts, preparedAccount{AccountNumber: record.AccountNumber, UserID: userID, InitialBalanceMinor: balance, Currency: currency, AccountType: record.AccountType})
		accountNumbers[record.AccountNumber] = struct{}{}
	}
	transactions := make([]preparedTransaction, 0, len(input.Transactions))
	for index, record := range input.Transactions {
		if _, ok := accountNumbers[record.FromAccount]; !ok && record.FromAccount != "EXTERNAL" {
			return nil, nil, fmt.Errorf("fixture transaction %d references unknown source account", index+1)
		}
		if _, ok := accountNumbers[record.ToAccount]; !ok && record.ToAccount != "EXTERNAL" {
			return nil, nil, fmt.Errorf("fixture transaction %d references unknown destination account", index+1)
		}
		amount, err := decimalToMinor(record.Amount)
		if err != nil || amount <= 0 {
			return nil, nil, fmt.Errorf("fixture transaction %d has invalid amount", index+1)
		}
		typeName := strings.ToLower(strings.TrimSpace(record.Type))
		if typeName != "deposit" && typeName != "withdrawal" && typeName != "transfer" {
			return nil, nil, fmt.Errorf("fixture transaction %d has invalid type", index+1)
		}
		status := strings.ToLower(strings.TrimSpace(record.Status))
		if status == "" {
			status = "completed"
		}
		if status != "completed" && status != "pending" && status != "failed" {
			return nil, nil, fmt.Errorf("fixture transaction %d has invalid status", index+1)
		}
		occurred, err := time.Parse(time.RFC3339, record.Timestamp)
		if err != nil {
			return nil, nil, fmt.Errorf("fixture transaction %d has invalid timestamp", index+1)
		}
		transactions = append(transactions, preparedTransaction{SourceKey: strconv.Itoa(index + 1), FromAccount: record.FromAccount, ToAccount: record.ToAccount, AmountMinor: amount, OperationType: typeName, Description: strings.TrimSpace(record.Description), OccurredAt: occurred.UTC(), Status: status, Currency: "USD"})
	}
	return accounts, transactions, nil
}

func decimalToMinor(value json.Number) (int64, error) {
	if strings.TrimSpace(value.String()) == "" {
		return 0, fmt.Errorf("amount is required")
	}
	rat, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return 0, fmt.Errorf("invalid decimal")
	}
	rat.Mul(rat, big.NewRat(100, 1))
	if rat.Denom().Cmp(big.NewInt(1)) != 0 || !rat.IsInt() || !rat.Num().IsInt64() {
		return 0, fmt.Errorf("amount has more than two decimals")
	}
	return rat.Num().Int64(), nil
}

// DuplicateReportFromFile loads a fixture and summarizes email collisions
// without requiring a database connection. It is intended for reconciliation
// before a migration is approved.
func DuplicateReportFromFile(filePath string) (DuplicateReport, error) {
	input, err := loadFixture(filePath)
	if err != nil {
		return DuplicateReport{}, err
	}
	return buildDuplicateReport(input), nil
}

// WriteDuplicateReport writes the safe reconciliation report as formatted JSON.
func WriteDuplicateReport(filePath string, report DuplicateReport) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode duplicate report: %w", err)
	}
	if err := os.WriteFile(filePath, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write duplicate report: %w", err)
	}
	return nil
}

type preparedUser struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FullName     string
	CreatedAt    time.Time
}

func loadFixture(filePath string) (fixture, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return fixture{}, fmt.Errorf("open fixture: %w", err)
	}
	defer file.Close()
	var input fixture
	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		return fixture{}, fmt.Errorf("decode fixture: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fixture{}, fmt.Errorf("fixture contains trailing data")
	}
	return input, nil
}

func validateUniqueEmails(users []userRecord) error {
	seen := make(map[string]struct{}, len(users))
	duplicates := 0
	for _, record := range users {
		email := auth.NormalizeEmail(record.Email)
		if _, exists := seen[email]; exists {
			duplicates++
			continue
		}
		seen[email] = struct{}{}
	}
	if duplicates > 0 {
		return fmt.Errorf("fixture contains %d duplicate email records", duplicates)
	}
	return nil
}

func buildDuplicateReport(input fixture) DuplicateReport {
	groups := make(map[string][]int)
	for index, record := range input.Users {
		email := auth.NormalizeEmail(record.Email)
		groups[email] = append(groups[email], index)
	}

	accountsByUser := make(map[string]int, len(input.Accounts))
	accountNumbersByUser := make(map[string][]string, len(input.Accounts))
	for _, account := range input.Accounts {
		accountsByUser[account.UserID]++
		accountNumbersByUser[account.UserID] = append(accountNumbersByUser[account.UserID], account.AccountNumber)
	}
	transactionsByAccount := make(map[string]struct{})
	for _, transaction := range input.Transactions {
		transactionsByAccount[transaction.FromAccount] = struct{}{}
		transactionsByAccount[transaction.ToAccount] = struct{}{}
	}

	duplicateEmails := make([]string, 0)
	for email, indexes := range groups {
		if len(indexes) > 1 {
			duplicateEmails = append(duplicateEmails, email)
		}
	}
	sort.Strings(duplicateEmails)

	report := DuplicateReport{DuplicateGroups: len(duplicateEmails), Conflicts: make([]DuplicateConflict, 0, len(duplicateEmails))}
	for groupNumber, email := range duplicateEmails {
		indexes := groups[email]
		conflict := DuplicateConflict{Group: groupNumber + 1, RecordIndexes: make([]int, 0, len(indexes)), SameFullName: true, SamePasswordField: true, SameCreatedAt: true, SameProfileFields: true}
		first := input.Users[indexes[0]]
		for _, index := range indexes {
			record := input.Users[index]
			conflict.RecordIndexes = append(conflict.RecordIndexes, index+1)
			if record.FullName != first.FullName {
				conflict.SameFullName = false
			}
			if record.Password != first.Password {
				conflict.SamePasswordField = false
			}
			if record.CreatedAt != first.CreatedAt {
				conflict.SameCreatedAt = false
			}
			if record.FullName != first.FullName || record.Password != first.Password || record.CreatedAt != first.CreatedAt {
				conflict.SameProfileFields = false
			}
			if accountsByUser[record.ID] > 0 {
				conflict.RecordsWithAccounts++
			}
			if userHasTransactions(record.ID, accountNumbersByUser, transactionsByAccount) {
				conflict.RecordsWithTransactions++
			}
		}
		report.DuplicateRecords += len(indexes)
		report.Conflicts = append(report.Conflicts, conflict)
	}
	return report
}

func userHasTransactions(userID string, accountNumbersByUser map[string][]string, transactionsByAccount map[string]struct{}) bool {
	for _, accountNumber := range accountNumbersByUser[userID] {
		if _, exists := transactionsByAccount[accountNumber]; exists {
			return true
		}
	}
	return false
}

func prepareUser(record userRecord, bcryptCost int) (preparedUser, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return preparedUser{}, fmt.Errorf("invalid fixture user id: %w", err)
	}
	validated, err := auth.ValidateRegistration(auth.RegisterInput{Email: record.Email, Password: record.Password, FullName: record.FullName})
	if err != nil {
		return preparedUser{}, fmt.Errorf("invalid fixture user %s", id)
	}
	hash, err := auth.HashPassword(validated.Password, bcryptCost)
	if err != nil {
		return preparedUser{}, err
	}
	createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
	if err != nil {
		return preparedUser{}, fmt.Errorf("invalid fixture user %s timestamp", id)
	}
	return preparedUser{ID: id, Email: validated.Email, PasswordHash: hash, FullName: validated.FullName, CreatedAt: createdAt.UTC()}, nil
}
