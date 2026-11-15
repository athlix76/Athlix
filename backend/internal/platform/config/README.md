# `internal/platform/config`

Package config loads the process bootstrap configuration from environment
variables.

Environment variables hold only NON-secret values and the NAMES of secrets.
Secret values (DB password, signing keys, partner keys) come from AWS Secrets
Manager via internal/platform/secrets. Business rules (fees, limits, switches)
live in the app_config table and are read through internal/platform/settings.

`Load` is for the app binaries (api, ws, worker): outside local it requires `REDIS_ADDR`
(not miniredis) and `MEDIA_BUCKET`. `LoadDBOnly` is for cmd/migrate and cmd/admincli, which
only touch MySQL. Both reject `DB_DSN` outside local. `REDIS_TLS` defaults to `true`
outside local and `false` locally. `DB_HOST_OVERRIDE` (host[:port]) replaces the host from
the `db` secret, for `LoadDBOnly` tools reached through an SSH tunnel; app binaries ignore it.


## Exported API

```go
type Config struct
func Load() (*Config, error)
func LoadDBOnly() (*Config, error)
func (c *Config) IsLocal() bool
func (c *Config) IsProd() bool
func (c *Config) SecretName(short string) string
type Env string
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/config`). Keep the package comment true and update this file when the API changes._
