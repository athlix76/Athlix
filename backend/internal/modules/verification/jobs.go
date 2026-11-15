package verification

import (
	"context"
	"log/slog"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/logger"
)

const (
	defaultSubjectHashRetentionDays = 180 // verification.subject_hash_retention_days
	purgeBatch                      = 500
	purgeMaxBatches                 = 20
)

// PurgeSubjectHashes is the verification.purge_subject_hash job. For users deleted more than
// verification.subject_hash_retention_days ago it clears identity_verifications.subject_hash
// and provider_ref (the last values that tie the account to a real identity). Rows of active
// or suspended users are never touched, so one identity still maps to at most one live
// account; a deleted user's identity is blocked from re-verifying until the retention ends.
// 500 rows per batch, at most 20 batches per run.
func (s *Service) PurgeSubjectHashes(ctx context.Context) error {
	days := s.d.Settings.Int(ctx, "verification.subject_hash_retention_days", defaultSubjectHashRetentionDays)
	if days < 1 {
		days = defaultSubjectHashRetentionDays
	}
	cutoff := s.d.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	var total int64
	for i := 0; i < purgeMaxBatches; i++ {
		ids, err := s.repo.subjectsToPurge(ctx, cutoff, purgeBatch)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			n, err := s.repo.clearSubjects(ctx, ids)
			if err != nil {
				return err
			}
			total += n
		}
		if len(ids) < purgeBatch {
			break
		}
	}
	logger.From(ctx).Info("verification.purge_subject_hash done", slog.Int64("rows", total))
	return nil
}

// subjectsToPurge returns ids of attempts of users deleted before cutoff that still hold a
// subject hash or provider ref.
func (r *repo) subjectsToPurge(ctx context.Context, cutoff time.Time, limit int) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT iv.id FROM identity_verifications iv JOIN users u ON u.id = iv.user_id WHERE u.status = 'deleted' AND u.deleted_at IS NOT NULL AND u.deleted_at < ? AND (iv.subject_hash IS NOT NULL OR iv.provider_ref IS NOT NULL) ORDER BY iv.id LIMIT ?`,
		cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// clearSubjects clears subject_hash and provider_ref of the given attempts. The join re-checks
// that the owner is still deleted.
func (r *repo) clearSubjects(ctx context.Context, ids []int64) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE identity_verifications iv JOIN users u ON u.id = iv.user_id SET iv.subject_hash = NULL, iv.provider_ref = NULL WHERE iv.id IN (`+db.Placeholders(len(ids))+`) AND u.status = 'deleted'`,
		db.Int64Args(ids)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
