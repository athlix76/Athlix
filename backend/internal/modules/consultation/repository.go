package consultation

import (
	"context"
	"database/sql"
	"encoding/json"

	"athlix/backend/internal/platform/db"
)

const requestCols = `id, coach_id, player_id, message, preferred_mode, status, created_at, updated_at`

// Owner columns used to scope a request to its caller. Never built from request input.
const (
	colCoach  = "coach_id"
	colPlayer = "player_id"
)

type repo struct{ db *sql.DB }

func (r *repo) listing(ctx context.Context, coachID int64) (listingRow, bool, error) {
	var l listingRow
	var desc, fee sql.NullString
	var modes []byte
	err := r.db.QueryRowContext(ctx, `SELECT offered, description, fee_note, modes FROM coach_consultations WHERE user_id = ?`, coachID).
		Scan(&l.Offered, &desc, &fee, &modes)
	if db.IsNoRows(err) {
		return listingRow{Modes: []string{}}, false, nil
	}
	if err != nil {
		return l, false, err
	}
	if desc.Valid {
		l.Description = &desc.String
	}
	if fee.Valid {
		l.FeeNote = &fee.String
	}
	if len(modes) > 0 {
		if err := json.Unmarshal(modes, &l.Modes); err != nil {
			return l, false, err
		}
	}
	if l.Modes == nil {
		l.Modes = []string{}
	}
	return l, true, nil
}

func (r *repo) upsertListing(ctx context.Context, coachID int64, l listingRow) error {
	modes, err := json.Marshal(l.Modes)
	if err != nil {
		return err
	}
	var desc, fee any
	if l.Description != nil {
		desc = *l.Description
	}
	if l.FeeNote != nil {
		fee = *l.FeeNote
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO coach_consultations (user_id, offered, description, fee_note, modes) VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE offered = VALUES(offered), description = VALUES(description), fee_note = VALUES(fee_note), modes = VALUES(modes)`,
		coachID, l.Offered, desc, fee, string(modes))
	return err
}

// pendingCounts returns the player's pending requests in total and to this coach. With
// forUpdate the rows (and the index gap) are locked so concurrent creates serialise.
func (r *repo) pendingCounts(ctx context.Context, q db.Querier, playerID, coachID int64, forUpdate bool) (total, toCoach int, err error) {
	sqlq := `SELECT id, coach_id FROM consultation_requests WHERE player_id = ? AND status = 'pending'`
	if forUpdate {
		sqlq += ` FOR UPDATE`
	}
	rows, err := q.QueryContext(ctx, sqlq, playerID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, cid int64
		if err := rows.Scan(&id, &cid); err != nil {
			return 0, 0, err
		}
		total++
		if cid == coachID {
			toCoach++
		}
	}
	return total, toCoach, rows.Err()
}

func (r *repo) insertRequest(ctx context.Context, q db.Querier, coachID, playerID int64, message, mode string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO consultation_requests (coach_id, player_id, message, preferred_mode, status) VALUES (?, ?, ?, ?, 'pending')`,
		coachID, playerID, message, mode)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type scanner interface{ Scan(dest ...any) error }

func scanRequest(s scanner) (requestRow, error) {
	var r requestRow
	err := s.Scan(&r.ID, &r.CoachID, &r.PlayerID, &r.Message, &r.PreferredMode, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	r.CreatedAt, r.UpdatedAt = r.CreatedAt.UTC(), r.UpdatedAt.UTC()
	return r, err
}

func (r *repo) getRequest(ctx context.Context, q db.Querier, id int64) (requestRow, error) {
	return scanRequest(q.QueryRowContext(ctx, `SELECT `+requestCols+` FROM consultation_requests WHERE id = ?`, id))
}

// getOwned loads request id only if ownerCol (colCoach / colPlayer) is userID, locked.
func (r *repo) getOwned(ctx context.Context, q db.Querier, id int64, ownerCol string, userID int64) (requestRow, error) {
	return scanRequest(q.QueryRowContext(ctx, `SELECT `+requestCols+` FROM consultation_requests WHERE id = ? AND `+ownerCol+` = ? FOR UPDATE`, id, userID))
}

func (r *repo) setStatus(ctx context.Context, q db.Querier, id int64, from, to string) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE consultation_requests SET status = ? WHERE id = ? AND status = ?`, to, id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// list pages the caller's requests by id DESC. ownerCol is colCoach or colPlayer.
func (r *repo) list(ctx context.Context, ownerCol string, userID int64, status string, beforeID int64, limit int) ([]requestRow, error) {
	q := `SELECT ` + requestCols + ` FROM consultation_requests WHERE ` + ownerCol + ` = ?`
	args := []any{userID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if beforeID > 0 {
		q += ` AND id < ?`
		args = append(args, beforeID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []requestRow
	for rows.Next() {
		rr, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rr)
	}
	return out, rows.Err()
}
