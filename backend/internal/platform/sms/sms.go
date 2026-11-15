// Package sms sends OTP SMS. The provider is chosen by app_config "auth.sms_provider".
// Implementations live here: Stub (logs only the masked number, never the OTP) and MSG91
// (internal/modules/auth wires it from the "msg91" secret).
package sms

import (
	"context"
	"log/slog"

	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/logger"
)

// Template ids a message can use.
const (
	TemplateLoginOTP    = "login_otp"
	TemplateGuardianOTP = "guardian_otp"
)

// Sender delivers an OTP. mobile is E.164 without '+', e.g. "919876543210".
type Sender interface {
	SendOTP(ctx context.Context, template, mobile, otp string) error
}

// Stub logs that an OTP was "sent" (masked number only). For local/dev.
type Stub struct{}

func (Stub) SendOTP(ctx context.Context, template, mobile, _ string) error {
	logger.From(ctx).Info("sms stub: otp sent", slog.String("template", template), slog.String("to", crypto.MaskMobile(crypto.Last4(mobile))))
	return nil
}
