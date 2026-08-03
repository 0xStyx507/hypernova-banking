package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ClientConfig controls the authenticated HTTP client used by assistant
// orchestration. Keeping this client separate makes provider and MCP tests
// independent from the running API process.
type ClientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Client calls the Hypernova MCP HTTP surface with a caller-provided opaque
// access token. Tokens are never stored on the client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// ToolError preserves the public MCP error contract across the assistant
// boundary. The chat handler can then return an actionable 403/404/409/422
// instead of disguising every tool failure as a generic 502.
type ToolError struct {
	Status  int
	Code    string
	Message string
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("MCP tool returned status %d (%s)", e.Status, e.Code)
}

// NewClient constructs a stateless MCP client.
func NewClient(config ClientConfig) *Client {
	return &Client{baseURL: strings.TrimRight(config.BaseURL, "/"), httpClient: config.HTTPClient}
}

// Call invokes one MCP tool and returns its JSON result without interpreting
// tool-specific data in this transport layer.
func (c *Client) Call(ctx context.Context, accessToken, name string, arguments any) (json.RawMessage, error) {
	if c == nil || c.baseURL == "" || strings.TrimSpace(accessToken) == "" || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("MCP client is not configured")
	}
	body, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return nil, fmt.Errorf("encode MCP call: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools/call", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create MCP request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call MCP tool: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read MCP response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var publicError struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(responseBody, &publicError)
		if strings.TrimSpace(publicError.Code) == "" {
			publicError.Code = "assistant_tool_error"
		}
		if strings.TrimSpace(publicError.Error) == "" {
			publicError.Error = "No pudimos completar la consulta del asistente."
		}
		return nil, &ToolError{Status: response.StatusCode, Code: publicError.Code, Message: publicError.Error}
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode MCP response: %w", err)
	}
	return envelope.Result, nil
}
