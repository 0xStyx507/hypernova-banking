package mcp

// ProtocolVersion identifies the authenticated HTTP tool contract. Increment
// it when tool names, argument schemas, or safety semantics change.
const ProtocolVersion = "hypernova-mcp-http/2"

// ReadOnlyToolNames is the server-side allowlist shared by the assistant and
// the MCP HTTP adapter. Mutating actions remain behind prepare/confirm.
var ReadOnlyToolNames = map[string]struct{}{
	"get_accounts":         {},
	"get_balance":          {},
	"get_transactions":     {},
	"search_transactions":  {},
	"get_cashflow_summary": {},
}
