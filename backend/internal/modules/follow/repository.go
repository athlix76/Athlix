package follow

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

// insertFollow adds follower → followee unless it exists or a block (either way) exists.
// It reports whether a row was added.
func (r *repo) insertFollow(ctx context.Context, q db.Querier, follower, followee int64) (bool, error) {
	res, err := q.ExecContext(ctx,
		`INSERT IGNORE INTO follows (follower_id, followee_id) SELECT ?, ? FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM blocks WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?))`,
		follower, followee, follower, followee, followee, follower)
	return affected(res, err)
}

// deleteFollow removes follower → followee and reports whether a row was removed.
func (r *repo) deleteFollow(ctx context.Context, q db.Querier, follower, followee int64) (bool, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM follows WHERE follower_id = ? AND followee_id = ?`, follower, followee)
	return affected(res, err)
}

func (r *repo) insertBlock(ctx context.Context, q db.Querier, blocker, blocked int64) error {
	_, err := q.ExecContext(ctx, `INSERT IGNORE INTO blocks (blocker_id, blocked_id) VALUES (?, ?)`, blocker, blocked)
	return err
}

func (r *repo) deleteBlock(ctx context.Context, blocker, blocked int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?`, blocker, blocked)
	return err
}

func affected(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// entry is one row of a list: the other user and when the relation was made.
type entry struct {
	UserID int64
	At     time.Time
}

// listKind selects one of the three lists. Each query is a fixed string; only values are bound.
type listKind int

const (
	kindFollowers listKind = iota
	kindFollowing
	kindBlocks
)

// listQuery is one list: the base query, the keyset condition and the order. All three are
// fixed strings; only values are bound.
type listQuery struct{ base, after, order string }

// The follow lists exclude users who are not active and anyone blocked either way with the
// viewer. Keyset order: created_at DESC, then the other user's id DESC.
var listQueries = map[listKind]listQuery{
	kindFollowers: {
		base:  `SELECT f.follower_id, f.created_at FROM follows f JOIN users u ON u.id = f.follower_id WHERE f.followee_id = ? AND u.status = 'active' AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = f.follower_id) OR (b.blocker_id = f.follower_id AND b.blocked_id = ?))`,
		after: ` AND (f.created_at < ? OR (f.created_at = ? AND f.follower_id < ?))`,
		order: ` ORDER BY f.created_at DESC, f.follower_id DESC LIMIT ?`,
	},
	kindFollowing: {
		base:  `SELECT f.followee_id, f.created_at FROM follows f JOIN users u ON u.id = f.followee_id WHERE f.follower_id = ? AND u.status = 'active' AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = f.followee_id) OR (b.blocker_id = f.followee_id AND b.blocked_id = ?))`,
		after: ` AND (f.created_at < ? OR (f.created_at = ? AND f.followee_id < ?))`,
		order: ` ORDER BY f.created_at DESC, f.followee_id DESC LIMIT ?`,
	},
	kindBlocks: {
		base:  `SELECT b.blocked_id, b.created_at FROM blocks b JOIN users u ON u.id = b.blocked_id WHERE b.blocker_id = ? AND u.status <> 'deleted'`,
		after: ` AND (b.created_at < ? OR (b.created_at = ? AND b.blocked_id < ?))`,
		order: ` ORDER BY b.created_at DESC, b.blocked_id DESC LIMIT ?`,
	},
}

// list runs one list for subject (whose followers / following / blocks), as seen by viewer.
func (r *repo) list(ctx context.Context, kind listKind, subject, viewer int64, after *entry, limit int) ([]entry, error) {
	lq := listQueries[kind]
	query := lq.base
	args := []any{subject}
	if kind != kindBlocks {
		args = append(args, viewer, viewer)
	}
	if after != nil {
		query += lq.after
		args = append(args, after.At, after.At, after.UserID)
	}
	query += lq.order
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.UserID, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- user deletion ----

// relations returns the ids on the other side of userID's follows, locked for the clean-up.
func (r *repo) relations(ctx context.Context, q db.Querier, userID int64) (followees, followers []int64, err error) {
	if followees, err = ids(ctx, q, `SELECT followee_id FROM follows WHERE follower_id = ? FOR UPDATE`, userID); err != nil {
		return nil, nil, err
	}
	followers, err = ids(ctx, q, `SELECT follower_id FROM follows WHERE followee_id = ? FOR UPDATE`, userID)
	return followees, followers, err
}

func ids(ctx context.Context, q db.Querier, query string, arg int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// removeAll deletes every follow and block of userID (both directions).
func (r *repo) removeAll(ctx context.Context, q db.Querier, userID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM follows WHERE follower_id = ? OR followee_id = ?`, userID, userID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `DELETE FROM blocks WHERE blocker_id = ? OR blocked_id = ?`, userID, userID)
	return err
}
