# `internal/platform/audit`

Package audit writes the admin audit trail (admin_audit_log) and config
change history (config_audit). Every admin write endpoint MUST call Admin;
every change to a config-like table MUST call Config in the same transaction.
Never put secrets or PII in details.


## Exported API

```go
func Admin(c *gin.Context, q db.Querier, action, targetType, targetID string, details any) error
func Config(ctx context.Context, q db.Querier, table, recordID string, old, new any, adminID int64) error
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/audit`). Keep the package comment true and update this file when the API changes._
