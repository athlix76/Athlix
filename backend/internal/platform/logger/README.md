# `internal/platform/logger`

Package logger builds the structured JSON logger used everywhere.

It redacts any attribute whose key looks like a secret or PII (token, otp,
password, mobile, phone, aadhaar, dob, email, authorization, secret, key,
signature, name_enc ...). Even so: log ids (userId, eventId, requestId),
never values. See docs/security.md.


## Exported API

```go
func From(ctx context.Context) *slog.Logger
func IsSensitiveKey(k string) bool
func New(level string) *slog.Logger
func With(ctx context.Context, l *slog.Logger) context.Context
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/logger`). Keep the package comment true and update this file when the API changes._
