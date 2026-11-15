# `internal/platform/ratelimit`

Package ratelimit is a Redis fixed-window limiter.


## Exported API

```go
type Limiter struct
func New(rdb *redis.Client) *Limiter
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/ratelimit`). Keep the package comment true and update this file when the API changes._
