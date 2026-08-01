// Package audit records security-relevant actions without storing secrets.
package audit

import (
	"context"
	"encoding/json"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is the subset shared by pgx pools and transactions.
type Executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Record writes one audit event. Details must contain metadata only; callers
// must never pass passwords, access tokens or refresh tokens.
func Record(ctx context.Context, executor Executor, userID *uuid.UUID, eventType string, details map[string]any, ipAddress, userAgent string) error {
	payload := []byte(`{}`)
	if details != nil {
		encoded, err := json.Marshal(details)
		if err != nil {
			return err
		}
		payload = encoded
	}

	var nullableUserID any
	if userID != nil {
		nullableUserID = *userID
	}
	var nullableIP any
	if parsed := net.ParseIP(strings.TrimSpace(ipAddress)); parsed != nil {
		nullableIP = parsed.String()
	}
	_, err := executor.Exec(ctx, `
		INSERT INTO audit_events (user_id, event_type, details, ip_address, user_agent)
		VALUES ($1, $2, $3, $4::inet, $5)
	`, nullableUserID, eventType, payload, nullableIP, truncate(userAgent, 512))
	return err
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
