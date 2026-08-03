package ledger

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
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
	debit := tigerbeetle.ToUint128(10)
	credit := tigerbeetle.ToUint128(20)
	first := hashRequest("transfer", debit, credit, 100, "USD")
	retry := hashRequest("transfer", debit, credit, 100, "USD")
	differentAmount := hashRequest("transfer", debit, credit, 101, "USD")
	if !bytes.Equal(first, retry) {
		t.Fatal("same financial intent must produce the same request hash")
	}
	if bytes.Equal(first, differentAmount) {
		t.Fatal("different amount must produce a different request hash")
	}
}

func TestHashAccountCreationIsStableAndCurrencyBound(t *testing.T) {
	first := hashAccountCreation("USD")
	retry := hashAccountCreation("USD")
	differentCurrency := hashAccountCreation("EUR")
	if !bytes.Equal(first, retry) {
		t.Fatal("same account creation intent must produce the same request hash")
	}
	if bytes.Equal(first, differentCurrency) {
		t.Fatal("different account currencies must produce different request hashes")
	}
}

func TestNormalizeCurrencyDefaultsToUSD(t *testing.T) {
	if got, err := normalizeCurrency(""); err != nil || got != "USD" {
		t.Fatalf("expected USD default, got %q, %v", got, err)
	}
	if _, err := normalizeCurrency("HNL"); err == nil {
		t.Fatal("expected unsupported currency to be rejected")
	}
}

func TestNormalizeTransferScopeDefaultsToOwn(t *testing.T) {
	if got, err := NormalizeTransferScope(""); err != nil || got != TransferScopeOwn {
		t.Fatalf("expected own default, got %q, %v", got, err)
	}
	if got, err := NormalizeTransferScope("EXTERNAL"); err != nil || got != TransferScopeExternal {
		t.Fatalf("expected external scope, got %q, %v", got, err)
	}
	if _, err := NormalizeTransferScope("beneficiary"); err != ErrInvalidInput {
		t.Fatalf("expected invalid transfer scope, got %v", err)
	}
}

func TestTransferScopeChangesIdempotencyIntent(t *testing.T) {
	debit := tigerbeetle.ToUint128(10)
	credit := tigerbeetle.ToUint128(20)
	own := hashRequestWithScope("transfer", debit, credit, 100, "USD", "own")
	external := hashRequestWithScope("transfer", debit, credit, 100, "USD", "external")
	if bytes.Equal(own, external) {
		t.Fatal("own and external transfer intents must not share a request hash")
	}
}

func TestSameTransferRejectsChangedFinancialIntent(t *testing.T) {
	transfer := tigerbeetle.Transfer{
		ID:              tigerbeetle.ToUint128(1),
		DebitAccountID:  tigerbeetle.ToUint128(10),
		CreditAccountID: tigerbeetle.ToUint128(20),
		Amount:          tigerbeetle.ToUint128(100),
		Ledger:          ledgerCode,
		Code:            transferCode,
	}
	if !sameTransfer(transfer, transfer) {
		t.Fatal("identical transfers must match")
	}
	changed := transfer
	changed.Amount = tigerbeetle.ToUint128(101)
	if sameTransfer(transfer, changed) {
		t.Fatal("a changed amount must not match the original transfer")
	}
}

func TestReconcilePendingOperationsRequiresInfrastructure(t *testing.T) {
	service := NewService(nil, nil)
	if _, err := service.ReconcilePendingOperations(context.Background()); err != ErrLedgerUnavailable {
		t.Fatalf("expected ledger unavailable, got %v", err)
	}
}
