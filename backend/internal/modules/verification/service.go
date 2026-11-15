package verification

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules/user"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Service implements DigiLocker / aggregator / mock verification.
type Service struct {
	d         *app.Deps
	repo      *repo
	providers map[string]provider
}

func newService(d *app.Deps, client *http.Client) *Service {
	return &Service{
		d:    d,
		repo: &repo{db: d.DB},
		providers: map[string]provider{
			ProviderDirect:     &direct{d: d, client: client},
			ProviderAggregator: &aggregator{d: d, client: client},
		},
	}
}

// ist is used for "today" and birthdays (no tzdata needed).
var ist = time.FixedZone("IST", 5*3600+1800)

const (
	subjectPurpose = "identity_subject"
	dobLayout      = "2006-01-02"
	attemptWindow  = 24 * time.Hour
)

var errStale = errors.New("verification: attempt no longer open")

func stateKey(state string) string {
	return "dl:state:" + hex.EncodeToString(crypto.SHA256(state))
}

func limitKey(userID int64) string { return "verify:" + strconv.FormatInt(userID, 10) }

// providerName is verification.provider; mock is never used in prod.
func (s *Service) providerName(ctx context.Context) string {
	p := s.d.Settings.String(ctx, "verification.provider", ProviderMock)
	if p == ProviderMock && s.d.Cfg.IsProd() {
		return ""
	}
	return p
}

func (s *Service) maxAttempts(ctx context.Context) int {
	return s.d.Settings.Int(ctx, "verification.max_attempts_per_day", 3)
}

func (s *Service) allow(ctx context.Context, userID int64) error {
	ok, err := s.d.Limiter.Allow(ctx, limitKey(userID), s.maxAttempts(ctx), attemptWindow)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.RateLimited("Too many verification attempts today. Try again tomorrow.")
	}
	return nil
}

func (s *Service) attemptsLeft(ctx context.Context, userID int64) int {
	used, err := s.d.Redis.Get(ctx, "rl:"+limitKey(userID)).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.From(ctx).Warn("verification attempts read", slog.Int64("userId", userID))
	}
	left := s.maxAttempts(ctx) - used
	if left < 0 {
		return 0
	}
	return left
}

// checkCanStart returns the user row when a new attempt may start.
func (s *Service) checkCanStart(ctx context.Context, userID int64) (userRow, error) {
	u, err := s.repo.user(ctx, s.d.DB, userID, false)
	if db.IsNoRows(err) {
		return u, httpx.NotFound("User")
	}
	if err != nil {
		return u, err
	}
	if u.VerificationStatus == "verified" {
		return u, httpx.InvalidState("Your identity is already verified.")
	}
	if !u.PrimaryRole.Valid {
		return u, httpx.InvalidState("Choose a role before verifying.")
	}
	return u, nil
}

// openAttempt expires older attempts, inserts a new one and marks the user pending.
func (s *Service) openAttempt(ctx context.Context, userID int64, provider, ref string) (int64, error) {
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if err := s.repo.expireInitiated(ctx, tx, userID); err != nil {
			return err
		}
		var err error
		if id, err = s.repo.insertAttempt(ctx, tx, userID, provider, ref); err != nil {
			return err
		}
		return s.repo.setPending(ctx, tx, userID)
	})
	if err == nil {
		s.d.Users.Invalidate(userID)
	}
	return id, err
}

// Start begins a DigiLocker attempt and returns the URL the app opens.
func (s *Service) Start(ctx context.Context, userID int64) (StartResult, error) {
	if _, err := s.checkCanStart(ctx, userID); err != nil {
		return StartResult{}, err
	}
	name := s.providerName(ctx)
	if name == ProviderMock {
		return StartResult{}, httpx.InvalidState("This environment uses the mock provider: call /verification/mock/complete.")
	}
	p, ok := s.providers[name]
	if !ok {
		logger.From(ctx).Error("verification provider not available", slog.String("provider", name))
		return StartResult{}, httpx.Upstream("Identity verification is unavailable right now.")
	}
	if err := s.allow(ctx, userID); err != nil {
		return StartResult{}, err
	}
	verifier, err := crypto.RandomToken(32)
	if err != nil {
		return StartResult{}, err
	}
	state, err := crypto.RandomToken(32)
	if err != nil {
		return StartResult{}, err
	}
	authURL, ref, err := p.Start(ctx, state, pkceChallenge(verifier))
	if err != nil {
		logger.From(ctx).Error("verification start failed", slog.Int64("userId", userID), slog.String("provider", name))
		return StartResult{}, httpx.Upstream("Identity verification is unavailable right now.")
	}
	attemptID, err := s.openAttempt(ctx, userID, name, ref)
	if err != nil {
		return StartResult{}, err
	}
	ttl := s.d.Settings.Seconds(ctx, "verification.state_ttl_seconds", 600)
	b, _ := json.Marshal(pendingState{UserID: userID, AttemptID: attemptID, Provider: name, Verifier: verifier, Ref: ref})
	if err := s.d.Redis.Set(ctx, stateKey(state), b, ttl).Err(); err != nil {
		return StartResult{}, err
	}
	return StartResult{AuthorizationURL: authURL, ExpiresAt: s.d.Now().Add(ttl).UTC()}, nil
}

// redeem consumes a state exactly once.
func (s *Service) redeem(ctx context.Context, state string) (pendingState, bool) {
	var ps pendingState
	raw, err := s.d.Redis.GetDel(ctx, stateKey(state)).Bytes()
	if err != nil || json.Unmarshal(raw, &ps) != nil || ps.UserID == 0 || ps.AttemptID == 0 {
		return ps, false
	}
	return ps, true
}

func (s *Service) deepLink(o outcome) string {
	return s.d.Cfg.AppDeepLinkBase + "verification/result?status=" + url.QueryEscape(o.Result) + "&reason=" + url.QueryEscape(o.Reason)
}

// Callback handles the provider redirect and returns the app deep link to 302 to.
func (s *Service) Callback(ctx context.Context, q url.Values) string {
	state := q.Get("state")
	if state == "" || len(state) > 256 {
		return s.deepLink(outcome{Result: ResultFailed, Reason: ReasonProviderError})
	}
	ps, ok := s.redeem(ctx, state)
	if !ok {
		logger.From(ctx).Warn("verification callback with unknown or used state")
		return s.deepLink(outcome{Result: ResultFailed, Reason: ReasonProviderError})
	}
	log := logger.From(ctx).With(slog.Int64("userId", ps.UserID), slog.Int64("attemptId", ps.AttemptID))
	p, ok := s.providers[ps.Provider]
	if !ok {
		return s.deepLink(s.failAttempt(ctx, ps.UserID, ps.AttemptID, ReasonProviderError))
	}
	code := q.Get("code")
	if q.Get("error") != "" || (ps.Provider == ProviderDirect && code == "") {
		return s.deepLink(s.failAttempt(ctx, ps.UserID, ps.AttemptID, ReasonCancelled))
	}
	id, err := p.Complete(ctx, code, ps.Verifier, ps.Ref)
	if errors.Is(err, errCancelled) {
		return s.deepLink(s.failAttempt(ctx, ps.UserID, ps.AttemptID, ReasonCancelled))
	}
	if err != nil {
		log.Error("verification provider fetch failed", slog.String("provider", ps.Provider))
		return s.deepLink(s.failAttempt(ctx, ps.UserID, ps.AttemptID, ReasonProviderError))
	}
	o, err := s.finish(ctx, ps.UserID, ps.AttemptID, id, 0, true)
	if err != nil {
		if !errors.Is(err, errStale) {
			log.Error("verification finish failed", slog.String("err", err.Error()))
		}
		return s.deepLink(outcome{Result: ResultFailed, Reason: ReasonProviderError})
	}
	return s.deepLink(o)
}

// failAttempt closes an open attempt as failed (no identity data is stored).
func (s *Service) failAttempt(ctx context.Context, userID, attemptID int64, reason string) outcome {
	o := outcome{Result: ResultFailed, Reason: reason}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		return s.failIn(ctx, tx, userID, attemptID, reason)
	})
	if err != nil {
		if !errors.Is(err, errStale) {
			logger.From(ctx).Error("verification fail attempt", slog.Int64("userId", userID), slog.String("err", err.Error()))
		}
		return o
	}
	s.after(ctx, userID, o)
	return o
}

func (s *Service) failIn(ctx context.Context, tx *sql.Tx, userID, attemptID int64, reason string) error {
	if err := s.lockOpen(ctx, tx, userID, attemptID); err != nil {
		return err
	}
	if err := s.repo.markFailed(ctx, tx, attemptID, reason); err != nil {
		return err
	}
	return s.repo.setUserVerification(ctx, tx, userID, "failed", false)
}

// lockOpen locks the attempt (must still be initiated) and the user (must not be verified).
func (s *Service) lockOpen(ctx context.Context, tx *sql.Tx, userID, attemptID int64) error {
	st, _, err := s.repo.attemptForUpdate(ctx, tx, attemptID, userID)
	if db.IsNoRows(err) || (err == nil && st != "initiated") {
		return errStale
	}
	if err != nil {
		return err
	}
	u, err := s.repo.user(ctx, tx, userID, true)
	if db.IsNoRows(err) || (err == nil && u.VerificationStatus == "verified") {
		return errStale
	}
	return err
}

// ageOn is the age in whole years on day now (IST).
func ageOn(dob, now time.Time) int {
	n := now.In(ist)
	age := n.Year() - dob.Year()
	if n.Month() < dob.Month() || (n.Month() == dob.Month() && n.Day() < dob.Day()) {
		age--
	}
	return age
}

// nameResembles is the verification.name_match_required rule: every word of the display
// name must appear in the verified name, ignoring case and punctuation, and initials match a
// word with that first letter.
func nameResembles(display, verified string) bool {
	words := func(s string) []string {
		return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 0x80
		})
	}
	have := words(verified)
	want := words(display)
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w || (len([]rune(w)) == 1 && strings.HasPrefix(h, w)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// finish applies the age, name and duplicate rules to a provider identity and stores the
// result. The address is matched to a district (matchAddress) or district is used as given
// (mock), then discarded. Events are published after commit.
func (s *Service) finish(ctx context.Context, userID, attemptID int64, id Identity, district int64, matchAddress bool) (outcome, error) {
	if matchAddress {
		district = 0
		if dist, ok := s.d.Eligibility.Places().MatchDistrict(ctx, id.StateName, id.DistrictName); ok {
			district = dist.ID
		}
	}
	id.StateName, id.DistrictName = "", "" // never stored, logged or sent

	now := s.d.Now()
	age := ageOn(id.DOB, now)
	minAge := s.d.Settings.Int(ctx, "auth.min_age_years", 10)
	minorAge := s.d.Settings.Int(ctx, "guardian.minor_age_years", 18)
	nameRule := s.d.Settings.Bool(ctx, "verification.name_match_required", false)

	var o outcome
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if err := s.lockOpen(ctx, tx, userID, attemptID); err != nil {
			return err
		}
		u, err := s.repo.user(ctx, tx, userID, false)
		if err != nil {
			return err
		}
		fail := func(reason string) error {
			o = outcome{Result: ResultFailed, Reason: reason}
			if err := s.repo.markFailed(ctx, tx, attemptID, reason); err != nil {
				return err
			}
			return s.repo.setUserVerification(ctx, tx, userID, "failed", false)
		}
		if age < minAge {
			return fail(ReasonTooYoung)
		}
		if nameRule && u.DisplayName.Valid && !nameResembles(u.DisplayName.String, id.Name) {
			return fail(ReasonNameMismatch)
		}
		hash := s.d.Crypto.Hash(subjectPurpose, id.SubjectID)
		owner, err := s.repo.subjectOwner(ctx, tx, hash)
		if err != nil {
			return err
		}
		if owner != 0 && owner != userID {
			return fail(ReasonAlreadyLinked)
		}
		nameEnc, err := s.d.Crypto.EncryptString(id.Name)
		if err != nil {
			return err
		}
		dobEnc, err := s.d.Crypto.EncryptString(id.DOB.Format(dobLayout))
		if err != nil {
			return err
		}
		var last4 sql.NullString
		if len(id.AadhaarLast4) == 4 && isDigits(id.AadhaarLast4) {
			last4 = sql.NullString{String: id.AadhaarLast4, Valid: true}
		}
		gender := id.Gender
		if gender == "" {
			gender = "U"
		}
		err = s.repo.markVerified(ctx, tx, attemptID, verifiedFields{
			SubjectHash: hash, Last4: last4, NameEnc: nameEnc, DOBEnc: dobEnc, Gender: gender, KeyVersion: s.d.Crypto.KeyVersion(),
		}, now)
		if db.IsDuplicate(err) {
			return fail(ReasonAlreadyLinked)
		}
		if err != nil {
			return err
		}
		isMinor := age < minorAge
		if err := s.repo.setVerified(ctx, tx, userID, isMinor); err != nil {
			return err
		}
		if err := s.repo.setBirthYear(ctx, tx, userID, id.DOB.Year()); err != nil {
			return err
		}
		if _, err := user.Advance(ctx, s.d, tx, userID); err != nil {
			return err
		}
		o = outcome{Result: ResultVerified, IsMinor: isMinor, NativeDistrictID: district}
		if isMinor {
			o.Result = ResultMinor
		}
		return nil
	})
	if err != nil {
		return outcome{}, err
	}
	s.after(ctx, userID, o)
	return o, nil
}

// after runs post-commit work: cache invalidation and verification.completed.
func (s *Service) after(ctx context.Context, userID int64, o outcome) {
	s.d.Users.Invalidate(userID)
	p := events.VerificationPayload{UserID: userID, Status: "failed"}
	if o.Result != ResultFailed {
		p.Status, p.IsMinor, p.NativeDistrictID = "verified", o.IsMinor, o.NativeDistrictID
	}
	if err := s.d.Events.Publish(ctx, events.New(events.VerificationCompleted, userID, p)); err != nil {
		logger.From(ctx).Error("publish verification.completed", slog.Int64("userId", userID), slog.String("err", err.Error()))
	}
}

// Mock completes a mock attempt (non-prod, verification.provider = mock only).
func (s *Service) Mock(ctx context.Context, userID int64, in mockBody) (Status, error) {
	if s.d.Cfg.IsProd() || s.providerName(ctx) != ProviderMock {
		return Status{}, httpx.NotFound("Route")
	}
	fields := map[string]string{}
	if !slices.Contains(MockOutcomes, in.Outcome) {
		fields["outcome"] = "Must be one of " + strings.Join(MockOutcomes, ", ") + "."
	}
	var district int64
	if in.NativeDistrictID != nil {
		if _, ok := s.d.Eligibility.Places().District(ctx, *in.NativeDistrictID); !ok || *in.NativeDistrictID <= 0 {
			fields["nativeDistrictId"] = "Unknown district."
		} else {
			district = *in.NativeDistrictID
		}
	}
	if len(fields) > 0 {
		return Status{}, httpx.Validation(fields)
	}
	u, err := s.checkCanStart(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	if err := s.allow(ctx, userID); err != nil {
		return Status{}, err
	}
	attemptID, err := s.openAttempt(ctx, userID, ProviderMock, "")
	if err != nil {
		return Status{}, err
	}
	minAge := s.d.Settings.Int(ctx, "auth.min_age_years", 10)
	minorAge := s.d.Settings.Int(ctx, "guardian.minor_age_years", 18)
	years := minorAge + 7
	switch in.Outcome {
	case "failed":
		s.failAttempt(ctx, userID, attemptID, ReasonProviderError)
		return s.Status(ctx, userID)
	case "duplicate":
		s.failAttempt(ctx, userID, attemptID, ReasonAlreadyLinked)
		return s.Status(ctx, userID)
	case "minor":
		years = minorAge - 1
	case "too_young":
		years = minAge - 1
	}
	name := "Mock User"
	if u.DisplayName.Valid && u.DisplayName.String != "" {
		name = u.DisplayName.String
	}
	now := s.d.Now().In(ist)
	id := Identity{
		SubjectID:    "mock:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(attemptID, 10),
		AadhaarLast4: "0000",
		Name:         name,
		DOB:          time.Date(now.Year()-years, now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1),
		Gender:       "U",
	}
	if _, err := s.finish(ctx, userID, attemptID, id, district, false); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, userID)
}

// Status is GET /verification/status.
func (s *Service) Status(ctx context.Context, userID int64) (Status, error) {
	u, err := s.repo.user(ctx, s.d.DB, userID, false)
	if db.IsNoRows(err) {
		return Status{}, httpx.NotFound("User")
	}
	if err != nil {
		return Status{}, err
	}
	st := Status{Status: u.VerificationStatus, IsMinor: u.IsMinor, AttemptsLeftToday: s.attemptsLeft(ctx, userID)}
	row, ok, err := s.repo.latest(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	if ok {
		st.Provider = &row.Provider
		if row.Status == "verified" {
			if row.Last4.Valid {
				st.AadhaarLast4 = &row.Last4.String
			}
			if row.VerifiedAt.Valid {
				t := row.VerifiedAt.Time.UTC()
				st.VerifiedAt = &t
			}
		}
		if u.VerificationStatus == "failed" && row.FailureReason.Valid {
			st.FailureReason = &row.FailureReason.String
		}
	}
	return st, nil
}

// MinorRecheck is the verification.minor_recheck job: users whose verified DOB shows they
// reached guardian.minor_age_years get is_minor = 0, guardian_consent_status not_required
// (unless revoked) and their onboarding step recomputed.
func (s *Service) MinorRecheck(ctx context.Context) error {
	minorAge := s.d.Settings.Int(ctx, "guardian.minor_age_years", 18)
	now := s.d.Now()
	const batch = 500
	var after int64
	cleared := 0
	for {
		rows, err := s.repo.minors(ctx, after, batch)
		if err != nil {
			return err
		}
		for _, m := range rows {
			after = m.UserID
			raw, err := s.d.Crypto.DecryptString(m.DOBEnc)
			if err != nil {
				logger.From(ctx).Warn("minor recheck: cannot read dob", slog.Int64("userId", m.UserID))
				continue
			}
			dob, err := time.Parse(dobLayout, raw)
			if err != nil || ageOn(dob, now) < minorAge {
				continue
			}
			changed := false
			err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
				var err error
				if changed, err = s.repo.clearMinor(ctx, tx, m.UserID); err != nil || !changed {
					return err
				}
				_, err = user.Advance(ctx, s.d, tx, m.UserID)
				return err
			})
			if err != nil {
				return err
			}
			if changed {
				cleared++
				s.d.Users.Invalidate(m.UserID)
			}
		}
		if len(rows) < batch {
			break
		}
	}
	logger.From(ctx).Info("verification.minor_recheck done", slog.Int("cleared", cleared))
	return nil
}

// onUserDeleted overwrites the verification ciphertext of a deleted user (idempotent).
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID == 0 {
		return nil // malformed: nothing to retry
	}
	n, err := s.repo.wipe(ctx, p.UserID)
	if err != nil {
		return err
	}
	logger.From(ctx).Info("verification data wiped", slog.Int64("userId", p.UserID), slog.Int64("rows", n))
	return nil
}

// BirthYear returns the year of the user's verified DOB (ok=false when not verified). The
// profile module uses it to set profiles.birth_year when the profile row is created.
func BirthYear(ctx context.Context, d *app.Deps, q db.Querier, userID int64) (int, bool, error) {
	if q == nil {
		q = d.DB
	}
	enc, err := (&repo{db: d.DB}).verifiedDOB(ctx, q, userID)
	if db.IsNoRows(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	raw, err := d.Crypto.DecryptString(enc)
	if err != nil {
		return 0, false, nil
	}
	t, err := time.Parse(dobLayout, raw)
	if err != nil {
		return 0, false, nil
	}
	return t.Year(), true, nil
}
