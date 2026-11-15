# `internal/platform/entitlements`

Package entitlements answers "does this user's active premium plan include
benefit X?".

Plans and benefits are DB config (premium_plans, premium_plan_benefits),
so what a plan includes changes without a deploy. Benefit keys are a closed list
(docs/config-system.md). Read-only over tables owned by modules/subscription.


## Exported API

```go
type Checker struct
func New(d *sql.DB) *Checker
func (c *Checker) Benefits(ctx context.Context, userID int64) (map[string]json.RawMessage, error)
func (c *Checker) Has(ctx context.Context, userID int64, key string) (bool, error)
func (c *Checker) Int(ctx context.Context, userID int64, key string, def int) (int, error)
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/entitlements`). Keep the package comment true and update this file when the API changes._
