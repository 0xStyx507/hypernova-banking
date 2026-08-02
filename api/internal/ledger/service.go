// Package ledger coordinates the financial API with TigerBeetle.
//
// PostgreSQL stores ownership, replay controls and an audit-friendly index;
// TigerBeetle remains the only source of truth for balances and transfers.
package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
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
