package ledger

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TransactionQuery describes bounded, read-only history filters. Amounts use
// minor units, matching the public financial contract.
type TransactionQuery struct {
	From      *time.Time
	To        *time.Time
	Type      string
	Direction string
	MinAmount int64
	MaxAmount int64
	Limit     uint32
}

// CashflowSummary is an account-scoped read model. String amounts prevent
// JSON consumers from applying floating-point arithmetic to money.
type CashflowSummary struct {
	Currency         string `json:"currency"`
	Credits          string `json:"credits"`
	Debits           string `json:"debits"`
	Net              string `json:"net"`
	TransactionCount int    `json:"transaction_count"`
}

// SearchHistory applies bounded filters to the ledger history read model. The
// ledger adapter intentionally reads at most maxHistory rows so a query cannot
// turn into an unbounded TigerBeetle scan.
func (s *Service) SearchHistory(ctx context.Context, userID uuid.UUID, accountID string, query TransactionQuery) ([]HistoryView, error) {
	limit := query.Limit
	if limit == 0 || limit > maxHistory {
		limit = maxHistory
	}
	items, err := s.History(ctx, userID, accountID, maxHistory, "")
	if err != nil {
		return nil, err
	}
	filtered := FilterHistory(items, query)
	if uint32(len(filtered)) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

// SummarizeHistory returns totals for the same bounded read model used by
// SearchHistory. It is deliberately derived from TigerBeetle facts, not from
// PostgreSQL's operation index.
func (s *Service) SummarizeHistory(ctx context.Context, userID uuid.UUID, accountID string, query TransactionQuery) (CashflowSummary, error) {
	items, err := s.SearchHistory(ctx, userID, accountID, query)
	if err != nil {
		return CashflowSummary{}, err
	}
	return SummarizeHistory(items, defaultCurrency), nil
}

// FilterHistory is kept pure so query semantics can be tested without a
// database or TigerBeetle process.
func FilterHistory(items []HistoryView, query TransactionQuery) []HistoryView {
	result := make([]HistoryView, 0, len(items))
	for _, item := range items {
		if query.Type != "" && item.Type != query.Type {
			continue
		}
		if query.Direction != "" && item.Direction != query.Direction {
			continue
		}
		if query.From != nil && item.CreatedAt.Before(query.From.UTC()) {
			continue
		}
		if query.To != nil && item.CreatedAt.After(query.To.UTC()) {
			continue
		}
		amount, err := strconv.ParseInt(item.Amount, 10, 64)
		if err != nil {
			continue
		}
		if query.MinAmount > 0 && amount < query.MinAmount {
			continue
		}
		if query.MaxAmount > 0 && amount > query.MaxAmount {
			continue
		}
		result = append(result, item)
	}
	return result
}

// SummarizeHistory aggregates credit and debit movements using checked
// int64 arithmetic. Malformed display data is ignored rather than inventing a
// financial value.
func SummarizeHistory(items []HistoryView, currency string) CashflowSummary {
	var credits, debits int64
	count := 0
	for _, item := range items {
		amount, err := strconv.ParseInt(strings.TrimSpace(item.Amount), 10, 64)
		if err != nil || amount <= 0 {
			continue
		}
		count++
		if item.Direction == "credit" {
			credits += amount
		} else if item.Direction == "debit" {
			debits += amount
		}
	}
	return CashflowSummary{
		Currency:         currency,
		Credits:          strconv.FormatInt(credits, 10),
		Debits:           strconv.FormatInt(debits, 10),
		Net:              strconv.FormatInt(credits-debits, 10),
		TransactionCount: count,
	}
}
