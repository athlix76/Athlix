package guardian

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/otp"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/sms"
)

// Service implements guardian consent for minors.
type Service struct {
	d    *app.Deps
	repo *repo
	otps *otp.Store
}

func newService(d *app.Deps) *Service {
	return &Service{d: d, repo: &repo{db: d.DB}, otps: otp.NewStore(d.Redis)}
}

// Redis keys. The challenge holds only the code hash and the owner id.
const (
	keyChallenge   = "guardian:otp:"
	keyCooldown    = "guardian:cooldown:"
	limitUser      = "guardian:user:"
	limitMobile    = "guardian:mobile:"
	fieldUser      = "u"
	maxNameRunes   = 80
	mobilePurpose  = "mobile" // same HMAC purpose as users.mobile_hash, so the two compare
	defaultOTPTTL  = 600
	defaultResend  = 30
	defaultPerHour = 5
)

func validName(s string) bool {
	n := utf8.RuneCountInString(s)
	if n < 2 || n > maxNameRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Request validates the guardian details, stores a pending consent (name and mobile
// encrypted) and sends the guardian_otp SMS to the guardian's mobile.
func (s *Service) Request(ctx context.Context, userID int64, in requestBody) (Ticket, error) {
	fields := map[string]string{}
	name := strings.TrimSpace(in.GuardianName)
	if !validName(name) {
		fields["guardianName"] = "must be 2 to 80 characters"
	}
	mobile, ok := otp.NormalizeMobile(in.GuardianMobile)
	if !ok {
		fields["guardianMobile"] = "must be a 10 digit Indian mobile number"
	}
	if !slices.Contains(Relations, in.Relation) {
		fields["relation"] = "must be one of " + strings.Join(Relations, ", ")
	}
	if in.PolicyDocumentID <= 0 {
		fields["policyDocumentId"] = "is required"
	}
	if len(fields) > 0 {
		return Ticket{}, httpx.Validation(fields)
	}

	u, err := s.repo.user(ctx, userID)
	if db.IsNoRows(err) {
		return Ticket{}, httpx.NotFound("User")
	}
	if err != nil {
		return Ticket{}, err
	}
	if u.VerificationStatus != "verified" || !u.IsMinor {
		return Ticket{}, httpx.InvalidState("Guardian consent is only needed for verified minors.")
	}
	if u.GuardianStatus == StatusGranted {
		return Ticket{}, httpx.InvalidState("Guardian consent is already granted.")
	}
	hash := s.d.Crypto.Hash(mobilePurpose, mobile)
	if subtle.ConstantTimeCompare(hash, u.MobileHash) == 1 {
		return Ticket{}, httpx.Validation(map[string]string{"guardianMobile": "must be your guardian's number, not your own"})
	}
	ok, err = s.repo.activeConsentPolicy(ctx, in.PolicyDocumentID)
	if err != nil {
		return Ticket{}, err
	}
	if !ok {
		return Ticket{}, httpx.Validation(map[string]string{"policyDocumentId": "must be the current guardian consent document"})
	}

	st := s.d.Settings
	ttl := st.Seconds(ctx, "guardian.consent_otp_ttl_seconds", defaultOTPTTL)
	cooldown := st.Seconds(ctx, "auth.otp.resend_cooldown_seconds", defaultResend)
	perHour := st.Int(ctx, "auth.otp.max_per_hour", defaultPerHour)
	uid := strconv.FormatInt(userID, 10)
	free, err := s.otps.Cooldown(ctx, keyCooldown+uid, cooldown)
	if err != nil {
		return Ticket{}, err
	}
	if !free {
		return Ticket{}, httpx.RateLimited("Please wait before asking for another code.")
	}
	for _, k := range []string{limitUser + uid, limitMobile + hex.EncodeToString(hash)} {
		allowed, err := s.d.Limiter.Allow(ctx, k, perHour, time.Hour)
		if err != nil {
			return Ticket{}, err
		}
		if !allowed {
			return Ticket{}, httpx.RateLimited("Too many codes requested. Try again later.")
		}
	}

	nameEnc, err := s.d.Crypto.EncryptString(name)
	if err != nil {
		return Ticket{}, err
	}
	mobileEnc, err := s.d.Crypto.EncryptString(mobile)
	if err != nil {
		return Ticket{}, err
	}
	last4 := crypto.Last4(mobile)
	var consentID int64
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		consentID, err = s.repo.insertPending(ctx, tx, newConsent{
			UserID: userID, NameEnc: nameEnc, MobileEnc: mobileEnc, MobileHash: hash, Last4: last4,
			Relation: in.Relation, PolicyID: in.PolicyDocumentID, KeyVersion: s.d.Crypto.KeyVersion(),
		})
		if err != nil {
			return err
		}
		return s.repo.markUserPending(ctx, tx, userID)
	})
	if err != nil {
		return Ticket{}, err
	}
	s.d.Users.Invalidate(userID)

	fixed := s.d.Cfg.IsLocal() && s.d.Cfg.DevFixedOTP != ""
	code := s.d.Cfg.DevFixedOTP
	if !fixed {
		if code, err = otp.Generate(st.Int(ctx, "auth.otp.length", 6)); err != nil {
			return Ticket{}, err
		}
	}
	cid := strconv.FormatInt(consentID, 10)
	if err := s.otps.Save(ctx, keyChallenge+cid, otp.Hash(cid, code), map[string]string{fieldUser: uid}, ttl); err != nil {
		return Ticket{}, err
	}
	if !fixed {
		if err := s.d.SMS.SendOTP(ctx, sms.TemplateGuardianOTP, otp.SMSNumber(mobile), code); err != nil {
			logger.From(ctx).Error("send guardian otp", slog.Int64("userId", userID), slog.Int64("consentId", consentID), slog.String("err", err.Error()))
			return Ticket{}, httpx.Upstream("Could not send the code. Try again.")
		}
	}
	return Ticket{
		ConsentID: consentID, GuardianMobileMasked: crypto.MaskMobile(last4),
		ExpiresInSeconds: int(ttl / time.Second), ResendAfterSeconds: int(cooldown / time.Second),
	}, nil
}

// Verify checks the guardian's OTP for the caller's consent, grants it, advances onboarding
// and returns Me. guardian.consent_granted is published after commit.
func (s *Service) Verify(ctx context.Context, userID int64, in verifyBody) (user.Me, error) {
	fields := map[string]string{}
	if in.ConsentID <= 0 {
		fields["consentId"] = "is required"
	}
	code := strings.TrimSpace(in.OTP)
	if code == "" || len(code) > 10 {
		fields["otp"] = "is required"
	}
	if len(fields) > 0 {
		return user.Me{}, httpx.Validation(fields)
	}
	// Ownership first: another user's consent id is 404 and never touches the challenge.
	st, err := s.repo.consentStatus(ctx, s.d.DB, in.ConsentID, userID, false)
	if db.IsNoRows(err) {
		return user.Me{}, httpx.NotFound("Consent")
	}
	if err != nil {
		return user.Me{}, err
	}
	if st != StatusPending {
		return user.Me{}, httpx.InvalidState("This consent request is no longer open.")
	}
	cid := strconv.FormatInt(in.ConsentID, 10)
	res, extra, err := s.otps.Check(ctx, keyChallenge+cid, cid, code, s.d.Settings.Int(ctx, "auth.otp.max_attempts", 5))
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

	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		st, err := s.repo.consentStatus(ctx, tx, in.ConsentID, userID, true)
		if err != nil {
			return err
		}
		if st != StatusPending {
			return httpx.InvalidState("This consent request is no longer open.")
		}
		if err := s.repo.grant(ctx, tx, in.ConsentID, userID, s.d.Now().UTC()); err != nil {
			return err
		}
		_, err = user.Advance(ctx, s.d, tx, userID)
		return err
	})
	if db.IsNoRows(err) {
		return user.Me{}, httpx.NotFound("Consent")
	}
	if err != nil {
		return user.Me{}, err
	}
	s.d.Users.Invalidate(userID)
	if err := s.d.Events.Publish(ctx, events.New(events.GuardianConsentGranted, userID, events.UserPayload{UserID: userID})); err != nil {
		logger.From(ctx).Error("publish guardian.consent_granted", slog.Int64("userId", userID), slog.String("err", err.Error()))
	}
	return user.LoadMe(ctx, s.d, userID)
}

// onUserDeleted overwrites the guardian name, mobile and hash of a deleted user (idempotent).
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID == 0 {
		return nil // malformed: nothing to retry
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	n, err := s.repo.wipe(ctx, p.UserID, random)
	if err != nil {
		return err
	}
	logger.From(ctx).Info("guardian data wiped", slog.Int64("userId", p.UserID), slog.Int64("rows", n))
	return nil
}
