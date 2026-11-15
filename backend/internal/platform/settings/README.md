# `internal/platform/settings`

Package settings reads runtime business configuration from the app_config table.

Every limit, switch, fee percentage etc. that the business may want to change
WITHOUT a deploy is an app_config row (documented in docs/config-system.md).
Values are cached in process for TTL (default 15s), so an admin change
takes effect within ~15 seconds on every Lambda. Typed config tables
(listing_fee_rules, premium_plans, ...) are read by their owning modules
directly.

Usage:

n := d.Settings.Int(ctx, "chat.group_max_members", 500)   // default used if key missing


## Exported API

```go
type Entry struct
type Store struct
func New(db *sql.DB, ttl time.Duration) *Store
func NewStatic(values map[string]any) *Store
func (s *Store) All(ctx context.Context) []Entry
func (s *Store) Bool(ctx context.Context, key string, def bool) bool
func (s *Store) Int(ctx context.Context, key string, def int) int
func (s *Store) Invalidate()
func (s *Store) JSON(ctx context.Context, key string, out any) bool
func (s *Store) Public(ctx context.Context) map[string]json.RawMessage
func (s *Store) Seconds(ctx context.Context, key string, def int) time.Duration
func (s *Store) Set(key string, v any)
func (s *Store) String(ctx context.Context, key, def string) string
func (s *Store) StringList(ctx context.Context, key string, def []string) []string
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/settings`). Keep the package comment true and update this file when the API changes._
