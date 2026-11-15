# `internal/platform/sms`

Package sms sends OTP SMS. The provider is chosen by app_config
"auth.sms_provider". Implementations live here: Stub (logs only the masked
number, never the OTP) and MSG91 (internal/modules/auth wires it from the
"msg91" secret).


## Exported API

```go
type MSG91 struct
func NewMSG91(s MSG91Secret) *MSG91
func (m *MSG91) SendOTP(ctx context.Context, template, mobile, otp string) error
type MSG91Secret struct
type Selector struct
func (s *Selector) SendOTP(ctx context.Context, template, mobile, otp string) error
type Sender interface
type Stub struct{}
func (Stub) SendOTP(ctx context.Context, template, mobile, _ string) error
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/sms`). Keep the package comment true and update this file when the API changes._
