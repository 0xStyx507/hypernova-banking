// Package seed imports the development fixture's identity data.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
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
	AccountNumber string `json:"account_number"`
	UserID        string `json:"user_id"`
}

type transactionRecord struct {
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
}

// Report summarizes work without printing credentials or fixture contents.
type Report struct {
	UsersProcessed       int
	AccountsDeferred     int
	TransactionsDeferred int
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
	if err := audit.Record(ctx, tx, nil, "seed_users", map[string]any{"users_processed": len(prepared)}, "", ""); err != nil {
		return Report{}, fmt.Errorf("audit seed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit seed: %w", err)
	}
	return Report{UsersProcessed: len(prepared), AccountsDeferred: len(input.Accounts), TransactionsDeferred: len(input.Transactions)}, nil
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
