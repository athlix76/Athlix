package auth

import (
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/modules/user"
)

type otpRequestBody struct {
	Mobile string `json:"mobile"`
}

// OTPTicket is the response of POST /auth/otp/request.
type OTPTicket struct {
	RequestID          string `json:"requestId"`
	ResendAfterSeconds int    `json:"resendAfterSeconds"`
	ExpiresInSeconds   int    `json:"expiresInSeconds"`
}

type otpVerifyBody struct {
	RequestID string `json:"requestId"`
	OTP       string `json:"otp"`
	DeviceID  string `json:"deviceId"`
}

// LoginResult is the response of POST /auth/otp/verify.
type LoginResult struct {
	Tokens    tokens.Pair `json:"tokens"`
	User      user.Me     `json:"user"`
	IsNewUser bool        `json:"isNewUser"`
}

type refreshBody struct {
	RefreshToken string `json:"refreshToken"`
}

// Redis keys (values hold hashes or ciphertext only, never a raw mobile or code).
const (
	keyChallenge = "otp:req:"      // + request id
	keyCooldown  = "otp:cooldown:" // + hex(mobile hash)
	limitMobile  = "otp:mobile:"   // + hex(mobile hash), ratelimit key
	limitIP      = "otp:ip:"       // + client ip, ratelimit key
	fieldMobile  = "m"             // base64 ciphertext of the normalised mobile
)
