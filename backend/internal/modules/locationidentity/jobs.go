package locationidentity

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/logger"
)

// ---- event handlers (idempotent: SQS delivers at least once) ----

func (s *Service) handlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.VerificationCompleted: s.onVerificationCompleted,
		events.UserDeleted:           s.onUserDeleted,
	}
}

// onVerificationCompleted creates the DigiLocker native identity when the Aadhaar address
// district was matched. Does nothing if the user already has a verified native.
func (s *Service) onVerificationCompleted(ctx context.Context, e events.Event) error {
	var p events.VerificationPayload
	if err := e.Decode(&p); err != nil {
		return nil // malformed payload: retrying will not help
	}
	if p.Status != statusVerified || p.NativeDistrictID <= 0 || p.UserID <= 0 {
		return nil
	}
	now := s.now()
	var id int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		existing, err := lockVerified(ctx, tx, p.UserID, typeNative)
		if err != nil || existing != nil {
			return err
		}
		stateID, err := districtState(ctx, tx, p.NativeDistrictID)
		if db.IsNoRows(err) {
			logger.From(ctx).Warn("native district not found", slog.Int64("userId", p.UserID), slog.Int64("districtId", p.NativeDistrictID))
			return nil
		}
		if err != nil {
			return err
		}
		id, err = insertIdentity(ctx, tx, newIdentity{
			UserID: p.UserID, Type: typeNative, DistrictID: p.NativeDistrictID, StateID: stateID,
			ProofType: proofDigiLocker, ProofHolder: holderSelf, Status: statusVerified, SubmittedAt: now, VerifiedAt: &now,
		})
		if db.IsDuplicate(err) { // created concurrently by a duplicate delivery
			id = 0
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	if id > 0 {
		s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: id, UserID: p.UserID, IdentityType: typeNative, DistrictID: p.NativeDistrictID, Status: statusVerified})
	}
	return nil
}

// onUserDeleted withdraws every open identity and deletes every location proof of the user.
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID <= 0 {
		return nil
	}
	var changed []changedRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		rows, err := scanChanged(ctx, tx, `SELECT `+changedCols+` FROM user_location_identities
			WHERE user_id = ? AND status IN ('pending','verified') FOR UPDATE`, p.UserID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			ok, err := setStatus(ctx, tx, r.ID, r.Status, statusWithdrawn)
			if err != nil {
				return err
			}
			if ok {
				changed = append(changed, r)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, r := range changed {
		s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: r.ID, UserID: r.UserID, IdentityType: r.Type, DistrictID: r.DistrictID, Status: statusWithdrawn, PrevStatus: r.Status})
	}
	uploads, err := s.repo.userProofUploads(ctx, p.UserID)
	if err != nil {
		return err
	}
	var errs []error
	for _, u := range uploads {
		if err := s.d.Media.Purge(ctx, u); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...) // retry; Purge is idempotent
	}
	return s.repo.clearUserProofs(ctx, p.UserID, s.now())
}

// ---- jobs (docs/architecture.md#scheduled-jobs; <= 500 rows per run, safe to re-run) ----

func (s *Service) jobs() map[string]app.JobFunc {
	return map[string]app.JobFunc{
		"location.expire_residence": s.expireResidence,
		"location.expiry_reminders": s.expiryReminders,
		"location.purge_proofs":     s.purgeProofs,
	}
}

func (s *Service) expireResidence(ctx context.Context) error {
	now := s.now()
	rows, err := s.repo.expiredResidences(ctx, now)
	if err != nil {
		return err
	}
	for _, r := range rows {
		ok, err := setStatus(ctx, s.d.DB, r.ID, statusVerified, statusExpired)
		if err != nil {
			return err
		}
		if ok {
			p := events.LocationIdentityPayload{IdentityID: r.ID, UserID: r.UserID, IdentityType: r.Type, DistrictID: r.DistrictID, Status: statusExpired, PrevStatus: statusVerified}
			if r.ExpiresAt.Valid {
				p.ExpiresAt = r.ExpiresAt.Time.UTC().Format(time.RFC3339)
			}
			s.publishChanged(ctx, 0, p)
		}
	}
	return nil
}

func (s *Service) expiryReminders(ctx context.Context) error {
	days := s.d.Settings.Int(ctx, "location.expiry_reminder_days", defReminderDays)
	if days <= 0 {
		return nil
	}
	now := s.now()
	rows, err := s.repo.expiringResidences(ctx, now, now.Add(time.Duration(days)*24*time.Hour))
	if err != nil {
		return err
	}
	for _, r := range rows {
		ok, err := s.repo.markReminded(ctx, r.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		p := events.LocationIdentityPayload{IdentityID: r.ID, UserID: r.UserID, IdentityType: r.Type, DistrictID: r.DistrictID, Status: r.Status}
		if r.ExpiresAt.Valid {
			p.ExpiresAt = r.ExpiresAt.Time.UTC().Format(time.RFC3339)
		}
		if err := s.d.Events.Publish(ctx, events.New(events.LocationIdentityExpiring, 0, p)); err != nil {
			logger.From(ctx).Error("publish location_identity.expiring", slog.Int64("identityId", r.ID), slog.String("err", err.Error()))
		}
	}
	return nil
}

func (s *Service) purgeProofs(ctx context.Context) error {
	days := s.d.Settings.Int(ctx, "location.proof_retention_days", defRetentionDays)
	if days < 0 {
		days = defRetentionDays
	}
	now := s.now()
	rows, err := s.repo.purgeCandidates(ctx, now.Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rows {
		if err := s.d.Media.Purge(ctx, r.UploadID); err != nil {
			logger.From(ctx).Error("purge location proof", slog.Int64("identityId", r.ID), slog.String("err", err.Error()))
			errs = append(errs, err)
			continue
		}
		if err := s.repo.markPurged(ctx, r.ID, r.UploadID, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
