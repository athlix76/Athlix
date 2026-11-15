// Package audit writes the admin audit trail (admin_audit_log) and config change history
// (config_audit). Every admin write endpoint MUST call Admin; every change to a config-like
// table MUST call Config in the same transaction. Never put secrets or PII in details.
package audit

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
)

// Admin records an admin action. details must hold ids/flags only.
func Admin(c *gin.Context, q db.Querier, action, targetType, targetID string, details any) error {
	var d any
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		d = string(b)
	}
	_, err := q.ExecContext(c.Request.Context(),
		`INSERT INTO admin_audit_log (admin_id, action, target_type, target_id, details, ip) VALUES (?, ?, ?, ?, ?, ?)`,
		authn.AdminID(c), action, db.NullString(targetType), db.NullString(targetID), d, c.ClientIP())
	return err
}

// Config records a change to a config table row. old/new are the row values (nil for create/delete).
func Config(ctx context.Context, q db.Querier, table, recordID string, old, new any, adminID int64) error {
	enc := func(v any) (any, error) {
		if v == nil {
			return nil, nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	}
	o, err := enc(old)
	if err != nil {
		return err
	}
	n, err := enc(new)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO config_audit (table_name, record_id, old_value, new_value, changed_by) VALUES (?, ?, ?, ?, ?)`,
		table, recordID, o, n, adminID)
	return err
}
