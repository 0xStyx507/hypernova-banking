package mcp

import "testing"

func TestNormalizeRequestAcceptsTransferIntent(t *testing.T) {
	payload, err := normalizeRequest(ActionRequest{
		ActionType:         "transfer",
		SourceAccountID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DestinationAccount: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Amount:             "1250",
		Currency:           "usd",
		Reason:             "Pago de prueba",
	})
	if err != nil {
		t.Fatalf("normalizeRequest() error = %v", err)
	}
	if payload.Currency != "USD" || payload.Amount != "1250" || payload.ActionType != "transfer" || payload.TransferType != "own" {
		t.Fatalf("unexpected normalized payload: %+v", payload)
	}
}

func TestNormalizeRequestAcceptsExternalTransferType(t *testing.T) {
	payload, err := normalizeRequest(ActionRequest{
		ActionType:         "transfer",
		SourceAccountID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DestinationAccount: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		TransferType:       "external",
		Amount:             "1250",
		Currency:           "USD",
	})
	if err != nil || payload.TransferType != "external" {
		t.Fatalf("expected external transfer type, payload=%+v, err=%v", payload, err)
	}
}

func TestNormalizeRequestRejectsAmbiguousIntent(t *testing.T) {
	_, err := normalizeRequest(ActionRequest{
		ActionType:         "transfer",
		SourceAccountID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		DestinationAccount: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Amount:             "1250",
		Currency:           "USD",
	})
	if err != ErrInvalidInput {
		t.Fatalf("normalizeRequest() error = %v, want %v", err, ErrInvalidInput)
	}
}

func TestNormalizeRequestRejectsFloatingPointAmount(t *testing.T) {
	_, err := normalizeRequest(ActionRequest{
		ActionType: "deposit",
		AccountID:  "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Amount:     "12.50",
		Currency:   "USD",
	})
	if err != ErrInvalidInput {
		t.Fatalf("normalizeRequest() error = %v, want %v", err, ErrInvalidInput)
	}
}
