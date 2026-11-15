package media

import (
	"context"
	"database/sql"
	"time"
)

type repo struct{ db *sql.DB }

// stalePending returns pending uploads created before cutoff, oldest first.
func (r *repo) stalePending(ctx context.Context, cutoff time.Time, afterID int64, limit int) ([]pendingRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, s3_key FROM media_uploads WHERE status = 'pending' AND created_at < ? AND id > ? ORDER BY id LIMIT ?`,
		cutoff, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingRow
	for rows.Next() {
		var p pendingRow
		if err := rows.Scan(&p.ID, &p.Key); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// deletePending removes the row only if it is still pending (a confirm in between wins).
func (r *repo) deletePending(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM media_uploads WHERE id = ? AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
