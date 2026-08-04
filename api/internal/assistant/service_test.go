package assistant

import (
	"context"
	"testing"
)

func TestLocalProviderPreparesDepositIntent(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "depositar 2500")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if response.FinancialAction == nil || response.FinancialAction.ActionType != "deposit" {
		t.Fatalf("expected a deposit action, got %#v", response.FinancialAction)
	}
	if response.FinancialAction.Amount != "2500" || !response.RequiresConfirmation {
		t.Fatalf("unexpected prepared intent: %#v", response.FinancialAction)
	}
}

func TestLocalProviderConvertsNaturalDollarAmountToMinorUnits(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "depositar USD 25.50")
	if err != nil || response.FinancialAction == nil || response.FinancialAction.Amount != "2550" {
		t.Fatalf("expected USD 25.50 to become 2550 minor units, response=%+v, err=%v", response, err)
	}
}

func TestLocalProviderConvertsSpanishNumberInOperationPhrase(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "depositar dos mil dólares")
	if err != nil || response.FinancialAction == nil || response.FinancialAction.Amount != "200000" {
		t.Fatalf("expected dos mil dólares to become 200000 minor units, response=%+v, err=%v", response, err)
	}
}

func TestLocalProviderUnderstandsCashflowQuery(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "cuánto gasté este mes")
	if err != nil || response.ReadOnlyTool != "get_cashflow_summary" {
		t.Fatalf("expected cashflow summary tool, response=%+v, err=%v", response, err)
	}
}

func TestLocalProviderRequiresExternalDestination(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "transferir 2500 a otra cuenta")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if response.FinancialAction != nil || response.Text == "" {
		t.Fatalf("expected a clear destination prompt, got %#v", response)
	}
}

func TestLocalProviderSelectsTransactionHistory(t *testing.T) {
	response, err := (LocalProvider{}).Complete(context.Background(), "muéstrame mis movimientos")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if response.ReadOnlyTool != "get_transactions" {
		t.Fatalf("ReadOnlyTool = %q, want get_transactions", response.ReadOnlyTool)
	}
}

func TestConversationKeepsDepositIntentForNumericReply(t *testing.T) {
	state := inferConversation("deposito", "account-1")
	if state == nil || state.ActionType != "deposit" || state.AccountID != "account-1" {
		t.Fatalf("unexpected conversation state: %#v", state)
	}
	if got := amountOnly("2333"); got != "2333" {
		t.Fatalf("amountOnly = %q, want 2333", got)
	}
}

func TestConversationAcceptsSpanishNumberAsAmount(t *testing.T) {
	if got := amountOnly("dos mil"); got != "200000" {
		t.Fatalf("amountOnly = %q, want 200000", got)
	}
}

func TestConversationPreservesAmountUntilAccountSelection(t *testing.T) {
	state := inferConversation("depositar 2500", "")
	if state == nil || state.ActionType != "deposit" || state.Amount != "2500" {
		t.Fatalf("conversation lost deposit amount: %#v", state)
	}
	if actionFromConversation(*state) != nil {
		t.Fatal("conversation prepared an action before an account was selected")
	}
	state.AccountID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	action := actionFromConversation(*state)
	if action == nil || action.Amount != "2500" || action.AccountID == "" {
		t.Fatalf("conversation did not prepare after account selection: %#v", action)
	}
}

func TestConversationAcceptsDestinationAfterSourcePrompt(t *testing.T) {
	state := ConversationState{ActionType: "transfer", SourceAccountID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Amount: "1000", TransferType: "own"}
	if actionFromConversation(state) != nil {
		t.Fatal("transfer became executable before destination selection")
	}
	state.DestinationAccount = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if actionFromConversation(state) == nil {
		t.Fatal("transfer did not become prepareable after destination selection")
	}
}
