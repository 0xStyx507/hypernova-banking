// Package ledger coordinates the financial API with TigerBeetle.
//
// PostgreSQL stores ownership, replay controls and an audit-friendly index;
// TigerBeetle remains the only source of truth for balances and transfers.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
)

const (
	defaultCurrency = "USD"
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
	ErrAccountNotEmpty     = errors.New("account balance must be zero before closing")
)

// TransferScope makes the destination rule explicit at the domain boundary.
// Own transfers require both accounts to belong to the authenticated user;
// external transfers require an active account owned by another user.
type TransferScope string

const (
	TransferScopeOwn      TransferScope = "own"
	TransferScopeExternal TransferScope = "external"
)

// NormalizeTransferScope applies the secure default for older clients that do
// not yet send the explicit transfer_type field.
func NormalizeTransferScope(value string) (TransferScope, error) {
	scope := TransferScope(strings.ToLower(strings.TrimSpace(value)))
	if scope == "" {
		return TransferScopeOwn, nil
	}
	if scope != TransferScopeOwn && scope != TransferScopeExternal {
		return "", ErrInvalidInput
	}
	return scope, nil
}

// Client is the small TigerBeetle surface used by this service. Keeping it
// local makes domain tests independent from a running ledger process.
type Client interface {
	CreateAccounts([]tigerbeetle.Account) ([]tigerbeetle.CreateAccountResult, error)
	CreateTransfers([]tigerbeetle.Transfer) ([]tigerbeetle.CreateTransferResult, error)
	LookupAccounts([]tigerbeetle.Uint128) ([]tigerbeetle.Account, error)
	LookupTransfers([]tigerbeetle.Uint128) ([]tigerbeetle.Transfer, error)
	GetAccountTransfers(tigerbeetle.AccountFilter) ([]tigerbeetle.Transfer, error)
	Nop() error
}

// RequestMetadata contains only non-secret request information for audit.
type RequestMetadata struct {
	IPAddress string
	UserAgent string
}

// Service exposes account and transfer use cases. Its methods are split into
// focused files by responsibility, while this type remains the package API.
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

// NewService creates a ledger service for the supported USD account model.
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
