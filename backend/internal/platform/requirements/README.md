# `internal/platform/requirements`

Package requirements answers "must a user with role R complete requirement X
before action G?" from the verification_requirements table (admin-editable,
see docs/config-system.md).

It is the ONE source of truth for gating: onboarding (gate app_access), event
creation (create_event), consultation listing (offer_consultation) and community
creation (create_community). Never hard-code these rules in a module.


## Exported API

```go
type Checker struct
func New(d *sql.DB, ttl time.Duration) *Checker
func NewStatic(rows map[string]bool) *Checker
func (c *Checker) Invalidate()
func (c *Checker) Required(ctx context.Context, role, requirement, gate string) bool
func (c *Checker) RequiredForAny(ctx context.Context, roles []string, requirement, gate string) bool
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/requirements`). Keep the package comment true and update this file when the API changes._
