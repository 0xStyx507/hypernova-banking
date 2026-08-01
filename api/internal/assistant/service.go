// Package assistant provides the provider-neutral chat orchestration layer.
// The default provider is deterministic and local so development never needs
// to send banking data to an external model.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hypernova-banking/api/internal/mcp"
)

var ErrInvalidMessage = errors.New("invalid assistant message")

// Provider is the only boundary an external LLM provider would implement.
type Provider interface {
	Complete(context.Context, string) (ProviderResponse, error)
}

// ProviderResponse contains model text and strictly named read-only tool
// suggestions. Mutating tools are never executed from this response.
type ProviderResponse struct {
	Text                 string
	ReadOnlyTool         string
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
		return ProviderResponse{Text: "Consultaré tu saldo HNL mediante una herramienta de solo lectura.", ReadOnlyTool: "get_accounts"}, nil
	}
	if strings.Contains(lower, "transfer") || strings.Contains(lower, "depósito") || strings.Contains(lower, "deposito") || strings.Contains(lower, "retiro") {
		return ProviderResponse{Text: "Puedo preparar esa operación, pero no moveré fondos sin mostrarte el detalle y recibir la confirmación exacta.", RequiresConfirmation: true}, nil
	}
	return ProviderResponse{Text: "Puedo consultar cuentas, saldos e historial HNL, o preparar una operación para tu confirmación."}, nil
}

// ChatResponse is the stable API response for assistant conversations.
type ChatResponse struct {
	Message              string          `json:"message"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	ReadOnlyData         json.RawMessage `json:"read_only_data,omitempty"`
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

// Chat obtains a response and, when requested by the provider, executes only
// the fixed read-only account lookup and balance lookup sequence.
func (s *Service) Chat(ctx context.Context, accessToken, message string) (ChatResponse, error) {
	if s == nil || s.provider == nil || s.mcp == nil {
		return ChatResponse{}, fmt.Errorf("assistant service is not configured")
	}
	response, err := s.provider.Complete(ctx, message)
	if err != nil {
		return ChatResponse{}, err
	}
	result := ChatResponse{Message: response.Text, RequiresConfirmation: response.RequiresConfirmation}
	if response.ReadOnlyTool != "get_accounts" {
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
	if err := json.Unmarshal(accounts, &accountResult); err != nil || len(accountResult.Items) == 0 {
		result.ReadOnlyData = accounts
		return result, nil
	}
	balance, err := s.mcp.Call(ctx, accessToken, "get_balance", map[string]string{"account_id": accountResult.Items[0].ID})
	if err != nil {
		return ChatResponse{}, err
	}
	result.ReadOnlyData = balance
	return result, nil
}
