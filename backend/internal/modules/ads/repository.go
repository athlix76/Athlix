package ads

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
)

const campaignCols = `id, advertiser_name, title, body, image_upload_id, target_url, placement, sport_id, state, priority, starts_at, ends_at, status`

// servingFilter selects ads that may be shown right now (status + time window).
const servingFilter = `status = 'active' AND starts_at <= ? AND ends_at > ?`

type repo struct{ db *sql.DB }

type scanner interface{ Scan(dest ...any) error }

func scanCampaign(s scanner) (Campaign, error) {
	var c Campaign
	var body, state sql.NullString
	var img, sport sql.NullInt64
	err := s.Scan(&c.ID, &c.AdvertiserName, &c.Title, &body, &img, &c.TargetURL, &c.Placement, &sport, &state, &c.Priority, &c.StartsAt, &c.EndsAt, &c.Status)
	if err != nil {
		return c, err
	}
	if body.Valid {
		c.Body = &body.String
	}
	if state.Valid {
		c.State = &state.String
	}
	if img.Valid {
		c.ImageUploadID = &img.Int64
	}
	if sport.Valid {
		c.SportID = &sport.Int64
	}
	c.StartsAt, c.EndsAt = c.StartsAt.UTC(), c.EndsAt.UTC()
	return c, nil
}

// serving returns ads for one placement, matched by context only (sport, state).
// sportID 0 / state "" match only untargeted ads.
func (r *repo) serving(ctx context.Context, now time.Time, placement string, sportID int64, state string, limit int) ([]Campaign, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+campaignCols+` FROM ad_campaigns
		WHERE `+servingFilter+` AND placement = ?
		  AND (sport_id IS NULL OR sport_id = ?) AND (state IS NULL OR state = ?)
		ORDER BY priority DESC, id DESC LIMIT ?`,
		now, now, placement, db.NullInt64(sportID), db.NullString(state), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// isServing reports whether ad id is servable now.
func (r *repo) isServing(ctx context.Context, now time.Time, id int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ad_campaigns WHERE id = ? AND `+servingFilter, id, now, now).Scan(&n)
	return n > 0, err
}

// bump adds one impression or click for (ad, IST date). No user data is stored.
func (r *repo) bump(ctx context.Context, id int64, date string, click bool) error {
	q := `INSERT INTO ad_daily_stats (ad_id, stat_date, impressions, clicks) VALUES (?, ?, 1, 0)
		ON DUPLICATE KEY UPDATE impressions = impressions + 1`
	if click {
		q = `INSERT INTO ad_daily_stats (ad_id, stat_date, impressions, clicks) VALUES (?, ?, 0, 1)
		ON DUPLICATE KEY UPDATE clicks = clicks + 1`
	}
	_, err := r.db.ExecContext(ctx, q, id, date)
	return err
}

func (r *repo) get(ctx context.Context, q db.Querier, id int64, forUpdate bool) (Campaign, error) {
	sqlq := `SELECT ` + campaignCols + ` FROM ad_campaigns WHERE id = ?`
	if forUpdate {
		sqlq += ` FOR UPDATE`
	}
	return scanCampaign(q.QueryRowContext(ctx, sqlq, id))
}

// list pages campaigns by id DESC; status "" means all.
func (r *repo) list(ctx context.Context, status string, beforeID int64, limit int) ([]Campaign, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+campaignCols+` FROM ad_campaigns
		WHERE (? = '' OR status = ?) AND (? = 0 OR id < ?)
		ORDER BY id DESC LIMIT ?`, status, status, beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func campaignArgs(c Campaign) []any {
	var body, state, img, sport any
	if c.Body != nil {
		body = *c.Body
	}
	if c.State != nil {
		state = *c.State
	}
	if c.ImageUploadID != nil {
		img = *c.ImageUploadID
	}
	if c.SportID != nil {
		sport = *c.SportID
	}
	return []any{c.AdvertiserName, c.Title, body, img, c.TargetURL, c.Placement, sport, state, c.Priority, c.StartsAt, c.EndsAt, c.Status}
}

func (r *repo) insert(ctx context.Context, q db.Querier, c Campaign, adminID int64) (int64, error) {
	args := append(campaignArgs(c), adminID)
	res, err := q.ExecContext(ctx, `INSERT INTO ad_campaigns
		(advertiser_name, title, body, image_upload_id, target_url, placement, sport_id, state, priority, starts_at, ends_at, status, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) update(ctx context.Context, q db.Querier, c Campaign) error {
	args := append(campaignArgs(c), c.ID)
	_, err := q.ExecContext(ctx, `UPDATE ad_campaigns SET advertiser_name = ?, title = ?, body = ?, image_upload_id = ?,
		target_url = ?, placement = ?, sport_id = ?, state = ?, priority = ?, starts_at = ?, ends_at = ?, status = ?
		WHERE id = ?`, args...)
	return err
}

func (r *repo) sportExists(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sports WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// stats returns per-day counters keyed by YYYY-MM-DD.
func (r *repo) stats(ctx context.Context, id int64, from, to string) (map[string]DayStat, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DATE_FORMAT(stat_date, '%Y-%m-%d'), impressions, clicks FROM ad_daily_stats
		WHERE ad_id = ? AND stat_date BETWEEN ? AND ? ORDER BY stat_date`, id, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DayStat{}
	for rows.Next() {
		var s DayStat
		if err := rows.Scan(&s.Date, &s.Impressions, &s.Clicks); err != nil {
			return nil, err
		}
		out[s.Date] = s
	}
	return out, rows.Err()
}

// endPast moves up to limit active / paused campaigns whose ends_at has passed to ended.
// Re-running it changes nothing for rows already ended.
func (r *repo) endPast(ctx context.Context, now time.Time, limit int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE ad_campaigns SET status = 'ended'
		WHERE status IN ('active', 'paused') AND ends_at <= ? ORDER BY id LIMIT ?`, now, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
