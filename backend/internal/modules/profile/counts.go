package profile

import (
	"context"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/userdir"
)

// The follow module keeps profiles.followers_count / following_count in step with the follows
// table through these helpers, inside its own transaction. The columns are unsigned, so a
// decrement never goes below zero.

// AdjustFollowers adds delta (+1 or -1) to userID's followers_count.
func AdjustFollowers(ctx context.Context, q db.Querier, userID int64, delta int) error {
	return adjust(ctx, q, userID, delta,
		`UPDATE profiles SET followers_count = followers_count + 1 WHERE user_id = ?`,
		`UPDATE profiles SET followers_count = followers_count - 1 WHERE user_id = ? AND followers_count > 0`)
}

// AdjustFollowing adds delta (+1 or -1) to userID's following_count.
func AdjustFollowing(ctx context.Context, q db.Querier, userID int64, delta int) error {
	return adjust(ctx, q, userID, delta,
		`UPDATE profiles SET following_count = following_count + 1 WHERE user_id = ?`,
		`UPDATE profiles SET following_count = following_count - 1 WHERE user_id = ? AND following_count > 0`)
}

func adjust(ctx context.Context, q db.Querier, userID int64, delta int, inc, dec string) error {
	query := inc
	if delta < 0 {
		query = dec
	} else if delta == 0 {
		return nil
	}
	_, err := q.ExecContext(ctx, query, userID)
	return err
}

// FollowersCount reads profiles.followers_count (0 when there is no profile).
func FollowersCount(ctx context.Context, q db.Querier, userID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT followers_count FROM profiles WHERE user_id = ?`, userID).Scan(&n)
	if db.IsNoRows(err) {
		return 0, nil
	}
	return n, err
}

// HideMinorCities removes city from the cards of minors the viewer does not follow (docs/api.md:
// "For minors, city is hidden from anyone who is not a follower"). The viewer's own card is
// never changed. Use it on every list of UserSummary built in Agent A modules.
func HideMinorCities(ctx context.Context, d *app.Deps, viewerID int64, cards []userdir.Summary) error {
	ids := []int64{}
	for _, c := range cards {
		if c.ID != viewerID && c.City != nil {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := d.DB.QueryContext(ctx,
		`SELECT u.id FROM users u WHERE u.is_minor = 1 AND u.id IN (`+db.Placeholders(len(ids))+`) AND NOT EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = ? AND f.followee_id = u.id)`,
		append(db.Int64Args(ids), viewerID)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	hide := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		hide[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range cards {
		if hide[cards[i].ID] {
			cards[i].City = nil
		}
	}
	return nil
}

// ListsVisible reports whether viewerID may see who targetID follows and is followed by: the
// same rule as the profile itself (public, followers when the viewer follows, owner always).
// A user without a profile row is treated as private.
func ListsVisible(ctx context.Context, d *app.Deps, viewerID, targetID int64) (bool, error) {
	if viewerID == targetID {
		return true, nil
	}
	var vis string
	err := d.DB.QueryRowContext(ctx, `SELECT visibility FROM profiles WHERE user_id = ?`, targetID).Scan(&vis)
	if db.IsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch vis {
	case "public":
		return true, nil
	case "followers":
		return d.Users.Follows(ctx, viewerID, targetID)
	}
	return false, nil
}
