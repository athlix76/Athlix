// Package push sends mobile push notifications. Production uses FCM HTTP v1 (fcm.go) with the
// `fcm` secret from Secrets Manager; tests use Fake; Nop is used when push is not configured.
//
// Never log or wrap into errors: device tokens, OAuth access tokens, the signed JWT, the
// private key, or FCM response bodies. Callers log user ids and device row ids only.
package push

import (
	"context"
	"errors"
	"sync"

	"athlix/backend/internal/platform/secrets"
)

// ErrUnregistered means the device token is no longer valid for this app (uninstalled, expired,
// wrong project). The caller deletes the token row.
var ErrUnregistered = errors.New("push: device token is not registered")

// ErrDisabled means push is not configured in this environment; callers skip silently.
var ErrDisabled = errors.New("push: disabled")

// Message is one push to one device. Data values must be strings (FCM rule) and hold
// deep-link ids only.
type Message struct {
	Token string
	Title string
	Body  string
	Data  map[string]string
}

// Sender delivers a push to a single device token.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Credentials is the shape of the `fcm` secret (docs/security.md).
type Credentials struct {
	ProjectID   string `json:"projectId"`
	ClientEmail string `json:"clientEmail"`
	PrivateKey  string `json:"privateKey"`
}

// FromSecrets reads the `fcm` secret by its full name (cfg.SecretName(app.SecretFCM)) and
// builds an FCM sender.
func FromSecrets(ctx context.Context, g secrets.Getter, name string) (Sender, error) {
	var c Credentials
	if err := g.GetJSON(ctx, name, &c); err != nil {
		return nil, err
	}
	return NewFCM(c)
}

// Nop is a Sender that always returns ErrDisabled.
type Nop struct{}

// Send implements Sender.
func (Nop) Send(context.Context, Message) error { return ErrDisabled }

// Fake records messages instead of sending them. Safe for concurrent use. Set Fail before use
// to make sends to a given token return an error (e.g. ErrUnregistered).
type Fake struct {
	mu   sync.Mutex
	Sent []Message
	Fail map[string]error
}

// Send implements Sender.
func (f *Fake) Send(_ context.Context, m Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.Fail[m.Token]; ok {
		return err
	}
	f.Sent = append(f.Sent, m)
	return nil
}

// Messages returns a copy of the recorded messages.
func (f *Fake) Messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.Sent...)
}
