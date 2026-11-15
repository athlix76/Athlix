package locationidentity

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/media"
	"athlix/backend/internal/platform/userdir"
)

// Service holds the location identity rules (docs/api.md#location-identity).
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

func (s *Service) now() time.Time { return s.d.Now().UTC() }

// ---- settings ----

func (s *Service) enabled(ctx context.Context) bool {
	return s.d.Settings.Bool(ctx, "location.enabled", defEnabled)
}

func (s *Service) proofTypes(ctx context.Context) ProofTypes {
	return ProofTypes{
		Native:    s.d.Settings.StringList(ctx, "location.native_proof_types", defNativeProofs),
		Residence: s.d.Settings.StringList(ctx, "location.residence_proof_types", defResidenceProofs),
	}
}

func (s *Service) guardianAllowed(ctx context.Context, u userdir.Basic) bool {
	return u.IsMinor && s.d.Settings.Bool(ctx, "location.minor_guardian_proof_allowed", defGuardianAllowed)
}

func (s *Service) cooldown(ctx context.Context) time.Duration {
	return time.Duration(s.d.Settings.Int(ctx, "location.residence_change_cooldown_days", defCooldownDays)) * 24 * time.Hour
}

func (s *Service) nativeChangeAllowed(ctx context.Context) bool {
	return s.d.Settings.Bool(ctx, "location.native_change_allowed", defNativeChange)
}

// ---- events ----

func (s *Service) publishChanged(ctx context.Context, actorID int64, p events.LocationIdentityPayload) {
	if err := s.d.Events.Publish(ctx, events.New(events.LocationIdentityChanged, actorID, p)); err != nil {
		logger.From(ctx).Error("publish location_identity.changed", slog.Int64("identityId", p.IdentityID), slog.String("err", err.Error()))
	}
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// purgeNow deletes a proof file after commit. On failure the location.purge_proofs job retries.
func (s *Service) purgeNow(ctx context.Context, identityID, uploadID int64) {
	if uploadID <= 0 {
		return
	}
	if err := s.d.Media.Purge(ctx, uploadID); err != nil {
		logger.From(ctx).Error("purge location proof", slog.Int64("identityId", identityID), slog.String("err", err.Error()))
		return
	}
	if err := s.repo.markPurged(ctx, identityID, uploadID, s.now()); err != nil {
		logger.From(ctx).Error("mark location proof purged", slog.Int64("identityId", identityID), slog.String("err", err.Error()))
	}
}

// ---- geo ----

// States is GET /geo/states.
func (s *Service) States(ctx context.Context) ([]State, error) {
	out, err := s.repo.states(ctx)
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	return out, nil
}

// Districts is GET /geo/districts.
func (s *Service) Districts(ctx context.Context, stateID int64) ([]District, error) {
	if stateID <= 0 {
		return nil, httpx.Validation(map[string]string{"stateId": "is required"})
	}
	out, err := s.repo.districts(ctx, stateID)
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	return out, nil
}

// ---- GET /me/location-identities ----

// Mine builds LocationIdentities for the caller.
func (s *Service) Mine(ctx context.Context, u userdir.Basic) (LocationIdentities, error) {
	rows, err := s.repo.userIdentities(ctx, u.ID)
	if err != nil {
		return LocationIdentities{}, httpx.Internal().Wrap(err)
	}
	out := LocationIdentities{ProofTypes: s.proofTypes(ctx), GuardianProofAllowed: s.guardianAllowed(ctx, u)}
	n, err := s.repo.activeNotice(ctx)
	switch {
	case err == nil:
		out.Notice = &n
	case !db.IsNoRows(err):
		return LocationIdentities{}, httpx.Internal().Wrap(err)
	}
	now := s.now()
	on := s.enabled(ctx)
	out.Native = s.slot(ctx, rows, typeNative, now, on)
	out.Residence = s.slot(ctx, rows, typeResidence, now, on)
	return out, nil
}

// slot applies the displayStatus rules to the user's rows (newest first) of one type.
func (s *Service) slot(ctx context.Context, rows []identityRow, typ string, now time.Time, on bool) IdentitySlot {
	var verified, pending, latest *identityRow
	for i := range rows {
		r := &rows[i]
		if r.Type != typ {
			continue
		}
		switch r.Status {
		case statusVerified:
			if verified == nil {
				verified = r
			}
		case statusPending:
			if pending == nil {
				pending = r
			}
		case statusExpired, statusRejected, statusRevoked:
			if latest == nil {
				latest = r
			}
		}
	}
	sl := IdentitySlot{DisplayStatus: "not_submitted"}
	if pending != nil {
		p := pending.toAPI()
		sl.Pending = &p
	}
	verifiedValid := verified != nil && (!verified.ExpiresAt.Valid || verified.ExpiresAt.Time.After(now))
	switch {
	case verifiedValid:
		sl.DisplayStatus = statusVerified
	case pending != nil:
		sl.DisplayStatus = statusPending
	case verified != nil: // verified but past expires_at (the expiry job has not run yet)
		sl.DisplayStatus = statusExpired
	case latest != nil && latest.Status == statusExpired:
		sl.DisplayStatus = statusExpired
	case latest != nil:
		sl.DisplayStatus = statusRejected // rejected or revoked
	}
	if verified != nil {
		c := verified.toAPI()
		sl.Current = &c
	} else if latest != nil {
		c := latest.toAPI()
		sl.Current = &c
	}
	sl.CanSubmit = on && pending == nil && !(typ == typeNative && verified != nil && !s.nativeChangeAllowed(ctx))
	if typ == typeResidence && verified != nil && verified.VerifiedAt.Valid {
		next := verified.VerifiedAt.Time.Add(s.cooldown(ctx)).UTC()
		if next.After(now) {
			sl.NextChangeAt = &next
		}
	}
	if verifiedValid && verified.ExpiresAt.Valid {
		days := int(math.Ceil(verified.ExpiresAt.Time.Sub(now).Hours() / 24))
		sl.ExpiresInDays = &days
	}
	return sl
}

// ---- POST /me/location-identities ----

// Submit creates a pending identity for the caller.
func (s *Service) Submit(ctx context.Context, u userdir.Basic, in LocationIdentityInput) (LocationIdentity, error) {
	if !s.enabled(ctx) {
		return LocationIdentity{}, httpx.InvalidState("Location identity is not available right now.")
	}
	in.IdentityType = strings.TrimSpace(in.IdentityType)
	in.ProofType = strings.TrimSpace(in.ProofType)
	in.ProofHolder = strings.TrimSpace(in.ProofHolder)
	if in.ProofHolder == "" {
		in.ProofHolder = holderSelf
	}
	if f := s.validateInput(ctx, u, in); len(f) > 0 {
		return LocationIdentity{}, httpx.Validation(f)
	}
	notice, err := s.repo.activeNotice(ctx)
	if err != nil && !db.IsNoRows(err) {
		return LocationIdentity{}, httpx.Internal().Wrap(err)
	}
	if err != nil || notice.PolicyDocumentID != in.NoticePolicyDocumentID {
		return LocationIdentity{}, httpx.Validation(map[string]string{"noticePolicyDocumentId": "must be the current location identity notice"})
	}
	dist, err := s.repo.districtWithState(ctx, in.DistrictID)
	if err != nil && !db.IsNoRows(err) {
		return LocationIdentity{}, httpx.Internal().Wrap(err)
	}
	if err != nil || !dist.Active {
		return LocationIdentity{}, httpx.Validation(map[string]string{"districtId": "is not an active district"})
	}

	now := s.now()
	var id int64
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		open, err := lockOpen(ctx, tx, u.ID, in.IdentityType)
		if err != nil {
			return err
		}
		for _, o := range open {
			if o.Status == statusPending {
				return httpx.Conflict("You already have a proof in review. Withdraw it first.")
			}
		}
		for _, o := range open {
			if o.Status != statusVerified {
				continue
			}
			if in.IdentityType == typeNative && !s.nativeChangeAllowed(ctx) {
				return httpx.InvalidState("Your native district is already verified and cannot be changed.")
			}
			if in.IdentityType == typeResidence && o.DistrictID != in.DistrictID && o.VerifiedAt.Valid {
				next := o.VerifiedAt.Time.Add(s.cooldown(ctx)).UTC()
				if next.After(now) {
					e := httpx.LimitReached("You changed your residence district recently.")
					e.Fields = map[string]string{"nextChangeAt": next.Format(time.RFC3339)}
					return e
				}
			}
		}
		used, err := proofInUse(ctx, tx, in.ProofUploadID)
		if err != nil {
			return err
		}
		if used {
			return httpx.Validation(map[string]string{"proofUploadId": "is already used"})
		}
		if err := s.d.Media.Claim(ctx, tx, u.ID, in.ProofUploadID, media.PurposeLocationProof); err != nil {
			if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrNotUploaded) || errors.Is(err, media.ErrMismatch) {
				return httpx.Validation(map[string]string{"proofUploadId": "must be your confirmed location proof upload"})
			}
			return err
		}
		id, err = insertIdentity(ctx, tx, newIdentity{
			UserID: u.ID, Type: in.IdentityType, DistrictID: dist.ID, StateID: dist.StateID,
			ProofType: in.ProofType, ProofHolder: in.ProofHolder, ProofUploadID: in.ProofUploadID,
			NoticeVersion: notice.Version, Status: statusPending, SubmittedAt: now,
		})
		if db.IsDuplicate(err) {
			return httpx.Conflict("You already have a proof in review. Withdraw it first.")
		}
		if err != nil {
			return err
		}
		return acceptNotice(ctx, tx, u.ID, notice.PolicyDocumentID)
	})
	if err != nil {
		if _, ok := httpx.As(err); ok {
			return LocationIdentity{}, err
		}
		return LocationIdentity{}, httpx.Internal().Wrap(err)
	}
	s.publishChanged(ctx, u.ID, events.LocationIdentityPayload{IdentityID: id, UserID: u.ID, IdentityType: in.IdentityType, DistrictID: dist.ID, Status: statusPending})
	row := identityRow{
		ID: id, UserID: u.ID, Type: in.IdentityType, DistrictID: dist.ID, DistrictName: dist.Name,
		StateID: dist.StateID, StateCode: dist.StateCode, StateName: dist.StateName, StateIsUT: dist.StateIsUT,
		ProofType: in.ProofType, ProofHolder: in.ProofHolder, Status: statusPending, SubmittedAt: now,
	}
	return row.toAPI(), nil
}

func (s *Service) validateInput(ctx context.Context, u userdir.Basic, in LocationIdentityInput) map[string]string {
	f := map[string]string{}
	pt := s.proofTypes(ctx)
	switch in.IdentityType {
	case typeNative:
		if !slices.Contains(pt.Native, in.ProofType) {
			f["proofType"] = "is not accepted for native identity"
		}
	case typeResidence:
		if !slices.Contains(pt.Residence, in.ProofType) {
			f["proofType"] = "is not accepted for residence identity"
		}
	default:
		f["identityType"] = "must be native or residence"
	}
	if in.DistrictID <= 0 {
		f["districtId"] = "is required"
	}
	if in.ProofUploadID <= 0 {
		f["proofUploadId"] = "is required"
	}
	if in.NoticePolicyDocumentID <= 0 {
		f["noticePolicyDocumentId"] = "is required"
	}
	switch in.ProofHolder {
	case holderSelf:
	case holderGuardian:
		if in.IdentityType != typeResidence || !s.guardianAllowed(ctx, u) {
			f["proofHolder"] = "guardian proof is allowed only for a minor's residence"
		}
	default:
		f["proofHolder"] = "must be self or guardian"
	}
	return f
}

// ---- DELETE /me/location-identities/:id ----

// Withdraw moves the caller's pending/verified row to withdrawn and deletes its proof.
func (s *Service) Withdraw(ctx context.Context, userID, id int64) error {
	var row ownedRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		row, err = lockOwned(ctx, tx, id, userID)
		if db.IsNoRows(err) {
			return httpx.NotFound("Location identity")
		}
		if err != nil {
			return err
		}
		if row.Status != statusPending && row.Status != statusVerified {
			return httpx.InvalidState("Only a pending or verified identity can be withdrawn.")
		}
		ok, err := setStatus(ctx, tx, id, row.Status, statusWithdrawn)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.InvalidState("Only a pending or verified identity can be withdrawn.")
		}
		return nil
	})
	if err != nil {
		if _, ok := httpx.As(err); ok {
			return err
		}
		return httpx.Internal().Wrap(err)
	}
	if row.ProofUploadID.Valid {
		s.purgeNow(ctx, id, row.ProofUploadID.Int64)
	}
	s.publishChanged(ctx, userID, events.LocationIdentityPayload{IdentityID: id, UserID: userID, IdentityType: row.Type, DistrictID: row.DistrictID, Status: statusWithdrawn, PrevStatus: row.Status})
	return nil
}
