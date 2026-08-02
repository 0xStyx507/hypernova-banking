-- Prepared actions are an approval boundary for assistant and MCP clients.
-- They describe intent only; balances and transfers remain in TigerBeetle.
CREATE TABLE mcp_prepared_actions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'ready',
    expires_at TIMESTAMPTZ NOT NULL,
    operation_id UUID REFERENCES ledger_operations(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    CONSTRAINT mcp_action_type_check CHECK (action_type IN ('deposit', 'withdrawal', 'transfer')),
    CONSTRAINT mcp_action_status_check CHECK (status IN ('ready', 'confirming', 'confirmed', 'cancelled', 'expired', 'failed'))
);

CREATE INDEX mcp_prepared_actions_user_created_idx
    ON mcp_prepared_actions (user_id, created_at DESC);
CREATE INDEX mcp_prepared_actions_expiry_idx
    ON mcp_prepared_actions (expires_at) WHERE status = 'ready';
