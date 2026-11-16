# `internal/platform/cache`

Package cache creates the Redis client. REDIS_ADDR=miniredis starts an
in-process Redis for local development and tests (never allowed outside
ENV=local). That miniredis follows the wall clock (`followWallClock`, a 250 ms ticker
calling `FastForward`) so TTLs expire as in a real Redis; without it an OTP resend
cooldown or rate-limit window would never end during a local run. Unit tests create
their own miniredis (`testutil`) and control time with `FastForward` themselves.

Redis holds only short-lived, non-secret state: OTP request records (hashed
OTP), rate-limit counters, WebSocket connection ids, ws tickets, DigiLocker
OAuth state.

Outside local the client dials TLS (`REDIS_TLS`, default `true` outside local), which
matches ElastiCache in-transit encryption mode `required`. `Options.Password` is an
optional AUTH token; if one is ever added it must come from a Secrets Manager secret,
never an environment variable or Terraform state.


## Exported API

```go
type Options struct{ Addr string; TLS bool; Password string }
func New(o Options) (*redis.Client, func(), error)
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/cache`). Keep the package comment true and update this file when the API changes._
