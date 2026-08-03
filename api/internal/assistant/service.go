// Package assistant provides the provider-neutral chat orchestration layer.
// The default provider is deterministic and local so development never needs
// to send banking data to an external model.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hypernova-banking/api/internal/mcp"
)

var ErrInvalidMessage = errors.New("invalid assistant message")

// Provider is the only boundary an external LLM provider would implement.
type Provider interface {
	Complete(context.Context, string) (ProviderResponse, error)
}

// ProviderResponse contains model text and strictly named tool suggestions.
// Mutating intents are only prepared; MCP confirmation remains a separate step.
type ProviderResponse struct {
	Text                 string
	ReadOnlyTool         string
	FinancialAction      *mcp.ActionRequest
	RequiresConfirmation bool
}

// LocalProvider is a deterministic provider for local runs and tests.
type LocalProvider struct{}

// Complete maps a small set of banking intents to safe responses. It is
// intentionally not presented as a financial decision-maker.
func (LocalProvider) Complete(_ context.Context, message string) (ProviderResponse, error) {
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 2000 {
		return ProviderResponse{}, ErrInvalidMessage
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "saldo") || strings.Contains(lower, "balance") {
		return ProviderResponse{Text: "Consultaré tu saldo USD mediante una herramienta de solo lectura.", ReadOnlyTool: "get_balance"}, nil
	}
	if strings.Contains(lower, "historial") || strings.Contains(lower, "transaccion") || strings.Contains(lower, "movimiento") {
		return ProviderResponse{Text: "Consultaré tus últimos movimientos USD.", ReadOnlyTool: "get_transactions"}, nil
	}
	if strings.Contains(lower, "cuenta") && !strings.Contains(lower, "transfer") && !strings.Contains(lower, "deposit") && !strings.Contains(lower, "depósito") && !strings.Contains(lower, "retiro") && !strings.Contains(lower, "retir") {
		return ProviderResponse{Text: "Consultaré tus cuentas activas.", ReadOnlyTool: "get_accounts"}, nil
	}
	if strings.Contains(lower, "transfer") || strings.Contains(lower, "depósito") || strings.Contains(lower, "deposit") || strings.Contains(lower, "retir") {
		action, err := parseFinancialAction(message)
		if err != nil {
			return ProviderResponse{Text: err.Error()}, nil
		}
		return ProviderResponse{Text: "Prepararé la operación para que revises el resumen y la confirmes con tu PIN.", FinancialAction: &action, RequiresConfirmation: true}, nil
	}
	return ProviderResponse{Text: "Puedo consultar cuentas, saldos e historial USD, o preparar depósitos, retiros y transferencias para tu confirmación."}, nil
}

var (
	uuidPattern       = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
	amountPattern     = regexp.MustCompile(`\b[1-9][0-9]{0,17}\b`)
	amountOnlyPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
)

func parseFinancialAction(message string) (mcp.ActionRequest, error) {
	lower := strings.ToLower(message)
	withoutUUIDs := uuidPattern.ReplaceAllString(lower, " ")
	amountMatch := amountPattern.FindString(withoutUUIDs)
	if amountMatch == "" {
		return mcp.ActionRequest{}, fmt.Errorf("Indica el monto en unidades menores, por ejemplo 2500 para USD 25.00.")
	}
	if _, err := strconv.ParseInt(amountMatch, 10, 64); err != nil {
		return mcp.ActionRequest{}, fmt.Errorf("El monto no es válido. Revísalo e inténtalo nuevamente.")
	}
	ids := uuidPattern.FindAllString(message, -1)
	request := mcp.ActionRequest{Amount: amountMatch, Currency: "USD"}
	switch {
	case strings.Contains(lower, "depósito") || strings.Contains(lower, "deposito") || strings.Contains(lower, "deposit"):
		request.ActionType = "deposit"
		if len(ids) > 0 {
			request.AccountID = ids[0]
		}
	case strings.Contains(lower, "retiro") || strings.Contains(lower, "retirar") || strings.Contains(lower, "withdraw"):
		request.ActionType = "withdrawal"
		if len(ids) > 0 {
			request.AccountID = ids[0]
		}
	case strings.Contains(lower, "transfer") || strings.Contains(lower, "envía") || strings.Contains(lower, "envia"):
		request.ActionType = "transfer"
		request.TransferType = "own"
		if strings.Contains(lower, "extern") || strings.Contains(lower, "otra cuenta") {
			request.TransferType = "external"
		}
		if len(ids) == 0 {
			return mcp.ActionRequest{}, fmt.Errorf("Indica la cuenta destino para preparar la transferencia.")
		}
		if len(ids) == 1 {
			request.DestinationAccount = ids[0]
			request.TransferType = "external"
		} else {
			request.SourceAccountID = ids[0]
			request.DestinationAccount = ids[1]
		}
	default:
		return mcp.ActionRequest{}, fmt.Errorf("No pude identificar la operación. Puedes pedir un depósito, retiro o transferencia.")
	}
	return request, nil
}

// ChatResponse is the stable API response for assistant conversations.
type ChatResponse struct {
	Message              string                `json:"message"`
	RequiresConfirmation bool                  `json:"requires_confirmation"`
	ReadOnlyData         json.RawMessage       `json:"read_only_data,omitempty"`
	Action               *mcp.Action           `json:"action,omitempty"`
	Conversation         *ConversationState    `json:"conversation,omitempty"`
	AccountOptions       []ConversationAccount `json:"account_options,omitempty"`
}

// ConversationAccount is a safe account choice for the chat UI. The server
// still revalidates ownership when the selected ID reaches MCP prepare.
type ConversationAccount struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Currency    string `json:"currency"`
}

// ConversationState contains only incomplete intent data. It is never an
// authorization artifact: the MCP prepare step revalidates ownership, amount,
// currency and destination before a financial action can be confirmed.
type ConversationState struct {
	ActionType         string                `json:"action,omitempty"`
	AccountID          string                `json:"account_id,omitempty"`
	SourceAccountID    string                `json:"source_account_id,omitempty"`
	DestinationAccount string                `json:"destination_account_id,omitempty"`
	TransferType       string                `json:"transfer_type,omitempty"`
	Amount             string                `json:"amount,omitempty"`
	AccountOptions     []ConversationAccount `json:"account_options,omitempty"`
}

// Service combines a provider with the authenticated MCP client.
type Service struct {
	provider Provider
	mcp      *mcp.Client
}

// NewService constructs provider-neutral chat orchestration.
func NewService(provider Provider, client *mcp.Client) *Service {
	return &Service{provider: provider, mcp: client}
}

// Chat obtains a response, executes bounded read-only queries, or prepares a
// persisted financial action. It never confirms or executes money movement.
func (s *Service) Chat(ctx context.Context, accessToken, message, selectedAccountID string, conversation *ConversationState) (ChatResponse, error) {
	if s == nil || s.provider == nil || s.mcp == nil {
		return ChatResponse{}, fmt.Errorf("assistant service is not configured")
	}
	response, err := s.provider.Complete(ctx, message)
	if err != nil {
		return ChatResponse{}, err
	}
	result := ChatResponse{Message: response.Text, RequiresConfirmation: response.RequiresConfirmation}
	if strings.Contains(strings.ToLower(strings.TrimSpace(message)), "cancel") || strings.Contains(strings.ToLower(strings.TrimSpace(message)), "cancelar") {
		if conversation != nil {
			return ChatResponse{Message: "Cancelé la operación pendiente."}, nil
		}
	}
	if response.FinancialAction == nil && conversation != nil {
		state := *conversation
		if accountID := uuidPattern.FindString(message); accountID != "" {
			switch {
			case state.ActionType == "transfer" && state.SourceAccountID == "":
				state.SourceAccountID = accountID
			case state.ActionType == "transfer" && state.DestinationAccount == "":
				state.DestinationAccount = accountID
			case (state.ActionType == "deposit" || state.ActionType == "withdrawal") && state.AccountID == "":
				state.AccountID = accountID
			}
		}
		if amount := amountOnly(message); amount != "" {
			state.Amount = amount
		}
		if action := actionFromConversation(state); action != nil {
			response.FinancialAction = action
			response.RequiresConfirmation = true
			result.RequiresConfirmation = true
			conversation = &state
		} else {
			return s.pendingConversation(ctx, accessToken, state, result)
		}
	}
	if response.FinancialAction == nil && response.ReadOnlyTool != "get_accounts" && response.ReadOnlyTool != "get_balance" && response.ReadOnlyTool != "get_transactions" {
		if pending := inferConversation(message, ""); pending != nil {
			return s.pendingConversation(ctx, accessToken, *pending, result)
		}
		return result, nil
	}
	accounts, err := s.mcp.Call(ctx, accessToken, "get_accounts", map[string]any{})
	if err != nil {
		return ChatResponse{}, err
	}
	var accountResult struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(accounts, &accountResult); err != nil {
		return ChatResponse{}, fmt.Errorf("decode assistant accounts: %w", err)
	}
	accountID := ""
	for _, account := range accountResult.Items {
		if account.ID == strings.TrimSpace(selectedAccountID) {
			accountID = account.ID
			break
		}
	}
	if accountID == "" && len(accountResult.Items) == 1 {
		accountID = accountResult.Items[0].ID
	}
	if response.FinancialAction != nil {
		actionRequest := *response.FinancialAction
		if actionRequest.ActionType == "deposit" || actionRequest.ActionType == "withdrawal" {
			// Use the account selected in the UI when the assistant omitted it.
			// Otherwise a simple "deposit 10" gets stuck asking for an account.
			if actionRequest.AccountID == "" {
				actionRequest.AccountID = accountID
			}
			if actionRequest.AccountID == "" {
				return s.pendingConversation(ctx, accessToken, ConversationState{ActionType: actionRequest.ActionType, Amount: actionRequest.Amount, TransferType: actionRequest.TransferType}, result)
			}
		}
		if actionRequest.ActionType == "transfer" && actionRequest.SourceAccountID == "" {
			actionRequest.SourceAccountID = accountID
			if actionRequest.SourceAccountID == "" {
				return s.pendingConversation(ctx, accessToken, ConversationState{ActionType: "transfer", Amount: actionRequest.Amount, DestinationAccount: actionRequest.DestinationAccount, TransferType: actionRequest.TransferType}, result)
			}
		}
		if actionRequest.ActionType == "transfer" && actionRequest.DestinationAccount == "" {
			result.Message = "Indica la cuenta destino para preparar la transferencia."
			result.Conversation = &ConversationState{ActionType: "transfer", SourceAccountID: actionRequest.SourceAccountID, TransferType: actionRequest.TransferType}
			return result, nil
		}
		if ((actionRequest.ActionType == "deposit" || actionRequest.ActionType == "withdrawal") && actionRequest.AccountID == "") || (actionRequest.ActionType == "transfer" && actionRequest.SourceAccountID == "") {
			result.Message = "Selecciona una cuenta activa antes de preparar la operación."
			result.Conversation = &ConversationState{ActionType: actionRequest.ActionType, TransferType: actionRequest.TransferType}
			return result, nil
		}
		prepared, err := s.mcp.Call(ctx, accessToken, "prepare_financial_action", actionRequest)
		if err != nil {
			return ChatResponse{}, err
		}
		var action mcp.Action
		if err := json.Unmarshal(prepared, &action); err != nil {
			return ChatResponse{}, fmt.Errorf("decode prepared assistant action: %w", err)
		}
		result.Action = &action
		result.Message = "La operación quedó preparada. Revisa el resumen y confírmala con tu PIN de cuatro dígitos."
		return result, nil
	}
	if response.ReadOnlyTool == "get_accounts" {
		result.ReadOnlyData = accounts
		return result, nil
	}
	if response.ReadOnlyTool != "get_transactions" && response.ReadOnlyTool != "get_balance" {
		return result, nil
	}
	if len(accountResult.Items) == 0 {
		result.ReadOnlyData = accounts
		result.Message = "No tienes cuentas activas disponibles para consultar."
		return result, nil
	}
	tool := "get_balance"
	if response.ReadOnlyTool == "get_transactions" {
		tool = "get_transactions"
	}
	arguments := map[string]any{"account_id": accountID}
	if tool == "get_transactions" {
		arguments["limit"] = 5
	}
	balance, err := s.mcp.Call(ctx, accessToken, tool, arguments)
	if err != nil {
		return ChatResponse{}, err
	}
	result.ReadOnlyData = balance
	return result, nil
}

func amountOnly(message string) string {
	value := strings.TrimSpace(message)
	if !amountOnlyPattern.MatchString(value) {
		return ""
	}
	return value
}

func inferConversation(message, selectedAccountID string) *ConversationState {
	lower := strings.ToLower(strings.TrimSpace(message))
	state := &ConversationState{AccountID: strings.TrimSpace(selectedAccountID), TransferType: "own"}
	switch {
	case strings.Contains(lower, "depósito") || strings.Contains(lower, "deposito") || strings.Contains(lower, "deposit"):
		state.ActionType = "deposit"
	case strings.Contains(lower, "retiro") || strings.Contains(lower, "retirar") || strings.Contains(lower, "withdraw"):
		state.ActionType = "withdrawal"
	case strings.Contains(lower, "transfer") || strings.Contains(lower, "envía") || strings.Contains(lower, "envia"):
		state.ActionType = "transfer"
		if strings.Contains(lower, "extern") || strings.Contains(lower, "otra cuenta") {
			state.TransferType = "external"
		}
	default:
		return nil
	}
	state.Amount = amountFromMessage(message)
	return state
}

func amountFromMessage(message string) string {
	match := amountPattern.FindString(uuidPattern.ReplaceAllString(strings.ToLower(message), " "))
	if match == "" {
		return ""
	}
	return match
}

func actionFromConversation(state ConversationState) *mcp.ActionRequest {
	if state.Amount == "" {
		return nil
	}
	switch state.ActionType {
	case "deposit", "withdrawal":
		if state.AccountID == "" {
			return nil
		}
	case "transfer":
		if state.SourceAccountID == "" || state.DestinationAccount == "" {
			return nil
		}
	default:
		return nil
	}
	return &mcp.ActionRequest{ActionType: state.ActionType, AccountID: state.AccountID, SourceAccountID: state.SourceAccountID, DestinationAccount: state.DestinationAccount, TransferType: state.TransferType, Amount: state.Amount, Currency: "USD"}
}

func (s *Service) pendingConversation(ctx context.Context, accessToken string, state ConversationState, result ChatResponse) (ChatResponse, error) {
	if state.ActionType != "transfer" && state.AccountID != "" && state.Amount == "" {
		result.Message = "Cuenta seleccionada. Indica el monto en unidades menores, por ejemplo 2500 para USD 25.00."
		result.Conversation = &state
		return result, nil
	}
	if state.ActionType == "transfer" && state.SourceAccountID != "" && state.DestinationAccount == "" {
		result.Message = "Cuenta de origen seleccionada. Indica la cuenta destino para continuar."
		result.Conversation = &state
		return result, nil
	}
	if state.ActionType == "transfer" && state.DestinationAccount == "" {
		result.Message = "Selecciona primero la cuenta de origen y luego indica la cuenta destino."
	} else if state.AccountID == "" || state.SourceAccountID == "" {
		result.Message = "Selecciona la cuenta que quieres utilizar para esta operación."
	} else if state.Amount == "" {
		result.Message = "Cuenta seleccionada. Indica el monto en unidades menores, por ejemplo 2500 para USD 25.00."
	}
	accounts, err := s.mcp.Call(ctx, accessToken, "get_accounts", map[string]any{})
	if err != nil {
		return ChatResponse{}, err
	}
	options, err := decodeAccountOptions(accounts)
	if err != nil {
		return ChatResponse{}, err
	}
	state.AccountOptions = options
	result.AccountOptions = options
	result.Conversation = &state
	return result, nil
}

func decodeAccountOptions(raw json.RawMessage) ([]ConversationAccount, error) {
	var envelope struct {
		Items []ConversationAccount `json:"items"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode assistant account options: %w", err)
	}
	return envelope.Items, nil
}
