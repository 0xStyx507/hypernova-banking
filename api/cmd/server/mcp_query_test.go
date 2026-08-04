package main

import "testing"

func TestTransactionQueryValidatesVersionedFilters(t *testing.T) {
	query, err := transactionQuery("transfer", "debit", "2026-08-01T00:00:00Z", "2026-08-31T23:59:59Z", "100", "5000", 20)
	if err != nil {
		t.Fatalf("transactionQuery() error = %v", err)
	}
	if query.Type != "transfer" || query.Direction != "debit" || query.MinAmount != 100 || query.MaxAmount != 5000 || query.Limit != 20 {
		t.Fatalf("unexpected query: %+v", query)
	}
}

func TestTransactionQueryRejectsInvalidDateAndRange(t *testing.T) {
	if _, err := transactionQuery("", "", "2026-08-01", "", "", "", 0); err == nil {
		t.Fatal("expected RFC3339 validation error")
	}
	if _, err := transactionQuery("", "", "", "", "500", "100", 0); err == nil {
		t.Fatal("expected amount range validation error")
	}
}
