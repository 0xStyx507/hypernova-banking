package ledger

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

func TestParseMinorAmountRejectsUnsafeValues(t *testing.T) {
	tests := []string{"", "0", "-1", "+1", "1.00", "1e3", "9223372036854775808"}
	for _, value := range tests {
		if _, err := parseMinorAmount(value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
	amount, err := parseMinorAmount("125050")
	if err != nil || amount != 125050 {
		t.Fatalf("expected minor amount 125050, got %d, %v", amount, err)
	}
}

func TestDemoDepositRequiresExplicitConfiguration(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.Deposit(context.Background(), uuid.New(), uuid.NewString(), "100", "key", RequestMetadata{})
	if err != ErrForbidden {
		t.Fatalf("expected demo deposit to be forbidden by default, got %v", err)
	}
}

func TestHashRequestChangesWithFinancialIntent(t *testing.T) {
	debit := types.ToUint128(10)
	credit := types.ToUint128(20)
	first := hashRequest("transfer", debit, credit, 100, "HNL")
	retry := hashRequest("transfer", debit, credit, 100, "HNL")
	differentAmount := hashRequest("transfer", debit, credit, 101, "HNL")
	if !bytes.Equal(first, retry) {
		t.Fatal("same financial intent must produce the same request hash")
	}
	if bytes.Equal(first, differentAmount) {
		t.Fatal("different amount must produce a different request hash")
	}
}

func TestNormalizeCurrencyDefaultsToHNL(t *testing.T) {
	if got, err := normalizeCurrency(""); err != nil || got != "HNL" {
		t.Fatalf("expected HNL default, got %q, %v", got, err)
	}
	if _, err := normalizeCurrency("USD"); err == nil {
		t.Fatal("expected unsupported currency to be rejected")
	}
}
