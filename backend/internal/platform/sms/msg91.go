package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// MSG91Secret is the JSON shape of the "msg91" secret.
type MSG91Secret struct {
	AuthKey            string `json:"authKey"`
	OTPTemplateID      string `json:"otpTemplateId"`
	GuardianTemplateID string `json:"guardianTemplateId"`
	SenderID           string `json:"senderId"`
}

// MSG91 sends OTPs through the MSG91 Flow API (DLT-registered templates). The auth key goes in
// a header; the OTP is in the request body and is never logged.
type MSG91 struct {
	s      MSG91Secret
	client *http.Client
	url    string
}

// NewMSG91 builds the sender.
func NewMSG91(s MSG91Secret) *MSG91 {
	return &MSG91{s: s, client: &http.Client{Timeout: 10 * time.Second}, url: "https://control.msg91.com/api/v5/flow/"}
}

func (m *MSG91) SendOTP(ctx context.Context, template, mobile, otp string) error {
	tid := m.s.OTPTemplateID
	if template == TemplateGuardianOTP {
		tid = m.s.GuardianTemplateID
	}
	body, _ := json.Marshal(map[string]any{
		"template_id": tid,
		"sender":      m.s.SenderID,
		"short_url":   "0",
		"recipients":  []map[string]string{{"mobiles": mobile, "otp": otp}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		return errors.New("sms: build request failed")
	}
	req.Header.Set("authkey", m.s.AuthKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		return errors.New("sms: provider unreachable")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return errors.New("sms: provider rejected request")
	}
	return nil
}

// Selector picks the provider named by app_config "auth.sms_provider" at send time, so the
// provider can be switched from the admin panel.
type Selector struct {
	Providers map[string]Sender
	Current   func(ctx context.Context) string
}

func (s *Selector) SendOTP(ctx context.Context, template, mobile, otp string) error {
	p, ok := s.Providers[s.Current(ctx)]
	if !ok {
		return errors.New("sms: provider not configured")
	}
	return p.SendOTP(ctx, template, mobile, otp)
}
