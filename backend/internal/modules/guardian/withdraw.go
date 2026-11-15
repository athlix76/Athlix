package guardian

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"athlix/backend/internal/modules/auth/otp"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/sms"
)

// Withdrawal of consent (docs/flows/minor-consent.md "Withdrawing consent").
// Self-service: the code goes to the guardian's mobile on the granted consent, so only the
// guardian can complete it. Support path: an admin with users.write revokes with a reason.
const (
	keyWithdraw         = "guardian:withdraw:"
	keyWithdrawCooldown = "guardian:withdraw:cooldown:"
	limitWithdraw       = "guardian:withdraw:user:"
	maxReasonRunes      = 255
)

// WithdrawRequest sends a code to the guardian of the caller's granted consent.
func (s *Service) WithdrawRequest(ctx context.Context, userID int64) (Ticket, error) {
	consentID, mobileEnc, err := s.repo.grantedConsent(ctx, userID)
	if db.IsNoRows(err) {
		return Ticket{}, httpx.InvalidState("There is no granted guardian consent to withdraw.")
	}
	if err != nil {
		return Ticket{}, err
	}
	mobile, err := s.d.Crypto.DecryptString(mobileEnc)
	if err != nil || mobile == "" {
		return Ticket{}, httpx.InvalidState("The guardian's number is not available. Contact support to withdraw consent.")
	}

	st := s.d.Settings
	ttl := st.Seconds(ctx, "guardian.consent_otp_ttl_seconds", defaultOTPTTL)
	cooldown := st.Seconds(ctx, "auth.otp.resend_cooldown_seconds", defaultResend)
	uid := strconv.FormatInt(userID, 10)
	free, err := s.otps.Cooldown(ctx, keyWithdrawCooldown+uid, cooldown)
	if err != nil {
		return Ticket{}, err
	}
	if !free {
		return Ticket{}, httpx.RateLimited("Please wait before asking for another code.")
	}
	allowed, err := s.d.Limiter.Allow(ctx, limitWithdraw+uid, st.Int(ctx, "auth.otp.max_per_hour", defaultPerHour), time.Hour)
	if err != nil {
		return Ticket{}, err
	}
	if !allowed {
		return Ticket{}, httpx.RateLimited("Too many codes requested. Try again later.")
	}

	fixed := s.d.Cfg.IsLocal() && s.d.Cfg.DevFixedOTP != ""
	code := s.d.Cfg.DevFixedOTP
	if !fixed {
		if code, err = otp.Generate(st.Int(ctx, "auth.otp.length", 6)); err != nil {
			return Ticket{}, err
		}
	}
	cid := strconv.FormatInt(consentID, 10)
	if err := s.otps.Save(ctx, keyWithdraw+cid, otp.Hash("withdraw:"+cid, code), map[string]string{fieldUser: uid}, ttl); err != nil {
		return Ticket{}, err
	}
	if !fixed {
		if err := s.d.SMS.SendOTP(ctx, sms.TemplateGuardianOTP, otp.SMSNumber(mobile), code); err != nil {
			logger.From(ctx).Error("send guardian withdraw otp", slog.Int64("userId", userID), slog.Int64("consentId", consentID), slog.String("err", err.Error()))
			return Ticket{}, httpx.Upstream("Could not send the code. Try again.")
		}
	}
	return Ticket{
		ConsentID: consentID, GuardianMobileMasked: crypto.MaskMobile(crypto.Last4(mobile)),
		ExpiresInSeconds: int(ttl / time.Second), ResendAfterSeconds: int(cooldown / time.Second),
	}, nil
}

// WithdrawVerify checks the guardian's code and revokes the consent. Returns Me (the
// onboarding step is back to guardian).
func (s *Service) WithdrawVerify(ctx context.Context, userID int64, in withdrawVerifyBody) (user.Me, error) {
	code := strings.TrimSpace(in.OTP)
	fields := map[string]string{}
	if in.ConsentID <= 0 {
		fields["consentId"] = "is required"
	}
	if code == "" || len(code) > 10 {
		fields["otp"] = "is required"
	}
	if len(fields) > 0 {
		return user.Me{}, httpx.Validation(fields)
	}
	st, err := s.repo.consentStatus(ctx, s.d.DB, in.ConsentID, userID, false)
	if db.IsNoRows(err) {
		return user.Me{}, httpx.NotFound("Consent")
	}
	if err != nil {
		return user.Me{}, err
	}
	if st != StatusGranted {
		return user.Me{}, httpx.InvalidState("This consent is not active.")
	}
	cid := strconv.FormatInt(in.ConsentID, 10)
	res, extra, err := s.otps.Check(ctx, keyWithdraw+cid, "withdraw:"+cid, code, s.d.Settings.Int(ctx, "auth.otp.max_attempts", 5))
	if err != nil {
		return user.Me{}, err
	}
	switch res {
	case otp.Wrong:
		return user.Me{}, httpx.Validation(map[string]string{"otp": "is incorrect"})
	case otp.Burnt, otp.Missing:
		return user.Me{}, httpx.Validation(map[string]string{"otp": "has expired, request a new code"})
	}
	if owner, err := otp.ParseInt64(extra, fieldUser); err != nil || owner != userID {
		return user.Me{}, httpx.NotFound("Consent")
	}
	if _, err := s.revoke(ctx, userID, reasonGuardianWithdrew, nil); err != nil {
		return user.Me{}, err
	}
	return user.LoadMe(ctx, s.d, userID)
}

// AdminRevoke revokes a user's granted consent for support (onTx writes the audit row).
func (s *Service) AdminRevoke(ctx context.Context, userID int64, in adminRevokeBody, onTx func(tx *sql.Tx) error) (Revoked, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxReasonRunes {
		return Revoked{}, httpx.Validation(map[string]string{"reason": "is required (at most 255 characters)"})
	}
	id, err := s.revoke(ctx, userID, reason, onTx)
	if err != nil {
		return Revoked{}, err
	}
	return Revoked{UserID: userID, ConsentID: id, GuardianConsentStatus: StatusRevoked}, nil
}

// revoke locks and revokes the user's granted consent, then publishes guardian.consent_revoked.
func (s *Service) revoke(ctx context.Context, userID int64, reason string, onTx func(tx *sql.Tx) error) (int64, error) {
	var consentID int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		consentID, err = s.repo.lockGrantedConsent(ctx, tx, userID)
		if db.IsNoRows(err) {
			return httpx.InvalidState("There is no granted guardian consent to withdraw.")
		}
		if err != nil {
			return err
		}
		if err := s.repo.revoke(ctx, tx, consentID, userID, reason, s.d.Now().UTC()); err != nil {
			return err
		}
		if onTx != nil {
			return onTx(tx)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.d.Users.Invalidate(userID)
	if err := s.d.Events.Publish(ctx, events.New(events.GuardianConsentRevoked, userID, events.UserPayload{UserID: userID})); err != nil {
		logger.From(ctx).Error("publish guardian.consent_revoked", slog.Int64("userId", userID), slog.String("err", err.Error()))
	}
	return consentID, nil
}
