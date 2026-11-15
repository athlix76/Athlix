package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/auth/otp"
	"athlix/backend/internal/modules/auth/tokens"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/sms"
)

// Service holds the login rules.
type Service struct {
	d      *app.Deps
	repo   repo
	otps   *otp.Store
	tokens *tokens.Store
}

func newService(d *app.Deps) *Service {
	return &Service{d: d, otps: otp.NewStore(d.Redis), tokens: tokens.New(d.DB, d.Now)}
}

func (s *Service) now() time.Time { return s.d.Now().UTC() }

func errSuspended() *httpx.Error {
	return httpx.Coded(403, httpx.CodeSuspended, "Your account is suspended.")
}

// RequestOTP validates the mobile, applies cooldown and rate limits, stores a challenge and
// sends the code. It never reveals whether the number has an account.
func (s *Service) RequestOTP(ctx context.Context, rawMobile, ip string) (OTPTicket, error) {
	mobile, ok := otp.NormalizeMobile(rawMobile)
	if !ok {
		return OTPTicket{}, httpx.Validation(map[string]string{"mobile": "must be a 10 digit Indian mobile number"})
	}
	st := s.d.Settings
	cooldown := st.Seconds(ctx, "auth.otp.resend_cooldown_seconds", 30)
	ttl := st.Seconds(ctx, "auth.otp.ttl_seconds", 300)
	perHour := st.Int(ctx, "auth.otp.max_per_hour", 5)
	// Separate, higher limit per IP: Indian mobile carriers put many users behind one CGNAT IP.
	perIPHour := st.Int(ctx, "auth.otp.max_per_ip_per_hour", 100)
	hx := hex.EncodeToString(s.d.Crypto.Hash("mobile", mobile))

	free, err := s.otps.Cooldown(ctx, keyCooldown+hx, cooldown)
	if err != nil {
		return OTPTicket{}, err
	}
	if !free {
		return OTPTicket{}, httpx.RateLimited("Please wait before asking for another code.")
	}
	for _, l := range []struct {
		key string
		max int
	}{{limitMobile + hx, perHour}, {limitIP + ip, perIPHour}} {
		allowed, err := s.d.Limiter.Allow(ctx, l.key, l.max, time.Hour)
		if err != nil {
			return OTPTicket{}, err
		}
		if !allowed {
			return OTPTicket{}, httpx.RateLimited("Too many codes requested. Try again later.")
		}
	}

	fixed := s.d.Cfg.IsLocal() && s.d.Cfg.DevFixedOTP != ""
	code := s.d.Cfg.DevFixedOTP
	if !fixed {
		if code, err = otp.Generate(st.Int(ctx, "auth.otp.length", 6)); err != nil {
			return OTPTicket{}, err
		}
	}
	enc, err := s.d.Crypto.EncryptString(mobile)
	if err != nil {
		return OTPTicket{}, err
	}
	rid := uuid.NewString()
	if err := s.otps.Save(ctx, keyChallenge+rid, otp.Hash(rid, code), map[string]string{fieldMobile: base64.StdEncoding.EncodeToString(enc)}, ttl); err != nil {
		return OTPTicket{}, err
	}
	if !fixed {
		if err := s.d.SMS.SendOTP(ctx, sms.TemplateLoginOTP, otp.SMSNumber(mobile), code); err != nil {
			logger.From(ctx).Error("send login otp", slog.String("requestId", rid), slog.String("err", err.Error()))
			return OTPTicket{}, httpx.Upstream("Could not send the code. Try again.")
		}
	}
	return OTPTicket{RequestID: rid, ResendAfterSeconds: int(cooldown / time.Second), ExpiresInSeconds: int(ttl / time.Second)}, nil
}

// VerifyOTP checks the code, finds or creates the user and starts a session.
func (s *Service) VerifyOTP(ctx context.Context, in otpVerifyBody) (LoginResult, error) {
	fields := map[string]string{}
	if _, err := uuid.Parse(in.RequestID); err != nil {
		fields["requestId"] = "is required"
	}
	code := strings.TrimSpace(in.OTP)
	if code == "" || len(code) > 10 {
		fields["otp"] = "is required"
	}
	if len(in.DeviceID) > 64 {
		fields["deviceId"] = "must be at most 64 characters"
	}
	if len(fields) > 0 {
		return LoginResult{}, httpx.Validation(fields)
	}
	res, extra, err := s.otps.Check(ctx, keyChallenge+in.RequestID, in.RequestID, code, s.d.Settings.Int(ctx, "auth.otp.max_attempts", 5))
	if err != nil {
		return LoginResult{}, err
	}
	switch res {
	case otp.Wrong:
		return LoginResult{}, httpx.Validation(map[string]string{"otp": "is incorrect"})
	case otp.Burnt, otp.Missing:
		return LoginResult{}, httpx.Validation(map[string]string{"otp": "has expired, request a new code"})
	}
	encMobile, err := base64.StdEncoding.DecodeString(extra[fieldMobile])
	if err != nil {
		return LoginResult{}, err
	}
	mobile, err := s.d.Crypto.DecryptString(encMobile)
	if err != nil {
		return LoginResult{}, err
	}

	var uid int64
	var isNew bool
	var refresh tokens.Issued
	login := func() error {
		return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
			hash := s.d.Crypto.Hash("mobile", mobile)
			id, status, err := s.repo.userByMobileHash(ctx, tx, hash)
			now := s.now()
			switch {
			case db.IsNoRows(err):
				enc, err := s.d.Crypto.EncryptString(mobile)
				if err != nil {
					return err
				}
				if id, err = s.repo.createUser(ctx, tx, enc, hash, crypto.Last4(mobile), now); err != nil {
					return err
				}
				isNew = true
			case err != nil:
				return err
			case status == "suspended":
				return errSuspended()
			case status != "active":
				return httpx.Unauthenticated()
			default:
				if err := s.repo.touchLogin(ctx, tx, id, now); err != nil {
					return err
				}
			}
			uid = id
			refresh, err = s.tokens.Issue(ctx, tx, tokens.SubjectUser, id, "", in.DeviceID, s.refreshExpiry(ctx))
			return err
		})
	}
	err = login()
	if db.IsDuplicate(err) { // a concurrent first login created the row; it now exists
		isNew = false
		err = login()
	}
	if err != nil {
		return LoginResult{}, err
	}
	pair, err := s.pair(ctx, uid, refresh)
	if err != nil {
		return LoginResult{}, err
	}
	me, err := user.LoadMe(ctx, s.d, uid)
	if err != nil {
		return LoginResult{}, err
	}
	if isNew {
		if err := s.d.Events.Publish(ctx, events.New(events.UserRegistered, uid, events.UserPayload{UserID: uid})); err != nil {
			logger.From(ctx).Error("publish user.registered", slog.Int64("userId", uid), slog.String("err", err.Error()))
		}
	}
	return LoginResult{Tokens: pair, User: me, IsNewUser: isNew}, nil
}

func (s *Service) refreshExpiry(ctx context.Context) time.Time {
	days := s.d.Settings.Int(ctx, "auth.refresh_token_ttl_days", 60)
	return s.now().Add(time.Duration(days) * 24 * time.Hour)
}

func (s *Service) pair(ctx context.Context, uid int64, refresh tokens.Issued) (tokens.Pair, error) {
	ttl := time.Duration(s.d.Settings.Int(ctx, "auth.access_token_ttl_minutes", 30)) * time.Minute
	access, exp, err := s.d.Tokens.IssueUser(uid, ttl)
	if err != nil {
		return tokens.Pair{}, err
	}
	return tokens.NewPair(access, exp, refresh), nil
}

// Refresh rotates a refresh token (sliding session).
func (s *Service) Refresh(ctx context.Context, raw string) (tokens.Pair, error) {
	rot, err := s.tokens.Rotate(ctx, tokens.SubjectUser, raw,
		func(time.Time) time.Time { return s.refreshExpiry(ctx) },
		func(ctx context.Context, q db.Querier, id int64) error {
			status, err := s.repo.userStatus(ctx, q, id)
			if db.IsNoRows(err) || status == "deleted" {
				return tokens.ErrInvalid
			}
			if err != nil {
				return err
			}
			if status == "suspended" {
				return errSuspended()
			}
			return nil
		})
	if errors.Is(err, tokens.ErrInvalid) {
		return tokens.Pair{}, httpx.Unauthenticated()
	}
	if err != nil {
		return tokens.Pair{}, err
	}
	return s.pair(ctx, rot.SubjectID, rot.Token)
}

// Logout revokes the session family of raw, or every session when raw is empty.
func (s *Service) Logout(ctx context.Context, uid int64, raw string) error {
	if raw == "" {
		return s.tokens.RevokeAll(ctx, nil, tokens.SubjectUser, uid)
	}
	return s.tokens.RevokeFamilyOf(ctx, nil, tokens.SubjectUser, uid, raw)
}

// cleanupTokens deletes refresh tokens expired or revoked more than 7 days ago, in batches.
func (s *Service) cleanupTokens(ctx context.Context) error {
	cutoff := s.now().Add(-7 * 24 * time.Hour)
	const batch, maxBatches = 500, 20
	for i := 0; i < maxBatches; i++ {
		n, err := s.tokens.Cleanup(ctx, cutoff, batch)
		if err != nil {
			return err
		}
		if n < batch {
			return nil
		}
	}
	return nil
}
