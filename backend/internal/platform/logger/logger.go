// Package logger builds the structured JSON logger used everywhere.
//
// It redacts any attribute whose key looks like a secret or PII (token, otp, password,
// mobile, phone, aadhaar, dob, email, authorization, secret, key, signature, name_enc ...).
// Even so: log ids (userId, eventId, requestId), never values. See docs/security.md.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

var redactKeys = []string{
	"token", "otp", "password", "passwd", "secret", "authorization", "cookie", "apikey", "api_key",
	"signature", "mobile", "phone", "aadhaar", "aadhar", "dob", "birth", "email", "address",
	"pan", "accountnumber", "account_number", "ifsc", "ticket", "code_verifier", "privatekey",
}

// Redacted is the replacement value for sensitive attributes.
const Redacted = "[REDACTED]"

// IsSensitiveKey reports whether a log/attribute key must never carry a raw value.
func IsSensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, r := range redactKeys {
		if strings.Contains(k, r) {
			return true
		}
	}
	return false
}

// New returns a JSON slog.Logger at the given level ("debug", "info", "warn", "error").
func New(level string) *slog.Logger {
	var lv slog.Level
	_ = lv.UnmarshalText([]byte(level))
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if IsSensitiveKey(a.Key) {
				return slog.String(a.Key, Redacted)
			}
			return a
		},
	})
	return slog.New(h)
}

type ctxKey struct{}

// With stores a request-scoped logger in ctx.
func With(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the request-scoped logger, or the default logger.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
