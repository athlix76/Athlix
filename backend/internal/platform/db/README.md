# `internal/platform/db`

Package db opens MySQL and provides transaction and error helpers.

ALL SQL uses ? bind parameters. Never build SQL with fmt.Sprintf/+ around a
value. Dynamic identifiers (sort column) must go through a hard-coded allowlist
map.


## Exported API

```go
func ForceUTC(dsn string) (string, error) // Open applies it: loc=UTC + session time_zone '+00:00'
func Int64Args(ids []int64) []any
func IsDuplicate(err error) bool
func IsNoRows(err error) bool
func NullInt64(v int64) any
func NullString(s string) any
func Open(dsn string, maxOpen int) (*sql.DB, error)
func Placeholders(n int) string
func WithTx(ctx context.Context, d *sql.DB, fn func(tx *sql.Tx) error) (err error)
type Querier interface
type Secret struct
func (s Secret) DSN() string
func (s Secret) WithHost(hostPort string) Secret // DB_HOST_OVERRIDE for migrate/admincli through a tunnel
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/db`). Keep the package comment true and update this file when the API changes._
