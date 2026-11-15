package feed

import (
	"context"
	"encoding/json"

	"athlix/backend/internal/platform/db"
)

// repo holds every SQL statement of the feed module. All values are bind parameters; the
// only string concatenation is of constant SQL fragments and db.Placeholders.
type repo struct{}

const postCols = `p.id, p.author_id, p.body, p.media_upload_ids, p.sport_id, p.event_id, p.visibility, p.like_count, p.comment_count, p.created_at`

// visibleTo is the visibility rule for viewer V (4 args, all V): the post is visible, its
// author is active and not blocked either way with V, and it is public, V's own, or V follows
// the author.
const visibleTo = ` p.status = 'visible'
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = p.author_id AND u.status = 'active')
  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = p.author_id) OR (b.blocker_id = p.author_id AND b.blocked_id = ?))
  AND (p.visibility = 'public' OR p.author_id = ? OR EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = ? AND f.followee_id = p.author_id))`

func visArgs(viewer int64) []any { return []any{viewer, viewer, viewer, viewer} }

type scanner interface{ Scan(dest ...any) error }

func scanPost(sc scanner) (postRow, error) {
	var p postRow
	var media []byte
	if err := sc.Scan(&p.ID, &p.AuthorID, &p.Body, &media, &p.SportID, &p.EventID, &p.Visibility, &p.LikeCount, &p.CommentCount, &p.CreatedAt); err != nil {
		return postRow{}, err
	}
	if len(media) > 0 {
		_ = json.Unmarshal(media, &p.MediaIDs)
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return p, nil
}

func queryPosts(ctx context.Context, q db.Querier, query string, args ...any) ([]postRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []postRow{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// visiblePost loads one post if viewer may see it (sql.ErrNoRows otherwise).
func (repo) visiblePost(ctx context.Context, q db.Querier, viewer, id int64) (postRow, error) {
	args := append([]any{id}, visArgs(viewer)...)
	return scanPost(q.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts p WHERE p.id = ? AND`+visibleTo, args...))
}

// listFollowing returns posts by people viewer follows plus viewer's own, newest first.
func (repo) listFollowing(ctx context.Context, q db.Querier, viewer, before int64, limit int) ([]postRow, error) {
	query := `SELECT ` + postCols + ` FROM posts p
	  WHERE (p.author_id = ? OR EXISTS (SELECT 1 FROM follows f2 WHERE f2.follower_id = ? AND f2.followee_id = p.author_id)) AND` + visibleTo
	args := append([]any{viewer, viewer}, visArgs(viewer)...)
	if before > 0 {
		query += ` AND p.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY p.id DESC LIMIT ?`
	args = append(args, limit)
	return queryPosts(ctx, q, query, args...)
}

// listDiscover returns public posts, newest first. With sports given, inSports selects posts
// in those sports, otherwise posts outside them (or without a sport).
func (repo) listDiscover(ctx context.Context, q db.Querier, viewer int64, sports []int64, inSports bool, before int64, limit int) ([]postRow, error) {
	query := `SELECT ` + postCols + ` FROM posts p WHERE p.visibility = 'public' AND` + visibleTo
	args := visArgs(viewer)
	if len(sports) > 0 {
		if inSports {
			query += ` AND p.sport_id IN (` + db.Placeholders(len(sports)) + `)`
		} else {
			query += ` AND (p.sport_id IS NULL OR p.sport_id NOT IN (` + db.Placeholders(len(sports)) + `))`
		}
		args = append(args, db.Int64Args(sports)...)
	}
	if before > 0 {
		query += ` AND p.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY p.id DESC LIMIT ?`
	args = append(args, limit)
	return queryPosts(ctx, q, query, args...)
}

// listByAuthor returns author's posts that viewer may see, newest first.
func (repo) listByAuthor(ctx context.Context, q db.Querier, viewer, author, before int64, limit int) ([]postRow, error) {
	query := `SELECT ` + postCols + ` FROM posts p WHERE p.author_id = ? AND` + visibleTo
	args := append([]any{author}, visArgs(viewer)...)
	if before > 0 {
		query += ` AND p.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY p.id DESC LIMIT ?`
	args = append(args, limit)
	return queryPosts(ctx, q, query, args...)
}

// mySports returns the viewer's sports (profile_sports plus the primary profile sport).
func (repo) mySports(ctx context.Context, q db.Querier, viewer int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT sport_id FROM profile_sports WHERE user_id = ?
	  UNION SELECT sport_id FROM profiles WHERE user_id = ? AND sport_id IS NOT NULL`, viewer, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// activeUser reports whether a user exists with status active (read-only on users).
func (repo) activeUser(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ? AND status = 'active'`, id).Scan(&n)
	return n > 0, err
}

// sportActive reports whether an active sport exists (read-only on sports).
func (repo) sportActive(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sports WHERE id = ? AND active = 1`, id).Scan(&n)
	return n > 0, err
}

// eventLinkable reports whether an event is published or completed (read-only on events).
func (repo) eventLinkable(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ? AND status IN ('published','completed')`, id).Scan(&n)
	return n > 0, err
}

func (repo) insertPost(ctx context.Context, q db.Querier, author int64, body string, media []byte, sportID, eventID int64, visibility string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO posts (author_id, body, media_upload_ids, sport_id, event_id, visibility) VALUES (?, ?, ?, ?, ?, ?)`,
		author, db.NullString(body), string(media), db.NullInt64(sportID), db.NullInt64(eventID), visibility)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// deletePost soft-deletes a visible post owned by author; false when none matched.
func (repo) deletePost(ctx context.Context, q db.Querier, id, author int64) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE posts SET status = 'deleted' WHERE id = ? AND author_id = ? AND status = 'visible'`, id, author)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// addLike inserts the like; true when a new row was created.
func (repo) addLike(ctx context.Context, q db.Querier, postID, userID int64) (bool, error) {
	res, err := q.ExecContext(ctx, `INSERT IGNORE INTO post_likes (post_id, user_id) VALUES (?, ?)`, postID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `UPDATE posts SET like_count = like_count + 1 WHERE id = ?`, postID)
	return err == nil, err
}

// removeLike deletes the like; true when a row was removed.
func (repo) removeLike(ctx context.Context, q db.Querier, postID, userID int64) (bool, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM post_likes WHERE post_id = ? AND user_id = ?`, postID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	_, err = q.ExecContext(ctx, `UPDATE posts SET like_count = IF(like_count > 0, like_count - 1, 0) WHERE id = ?`, postID)
	return err == nil, err
}

func (repo) likeCount(ctx context.Context, q db.Querier, postID int64) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT like_count FROM posts WHERE id = ?`, postID).Scan(&n)
	return n, err
}

// likedBy returns which of postIDs viewer has liked.
func (repo) likedBy(ctx context.Context, q db.Querier, viewer int64, postIDs []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(postIDs) == 0 {
		return out, nil
	}
	args := append([]any{viewer}, db.Int64Args(postIDs)...)
	rows, err := q.QueryContext(ctx, `SELECT post_id FROM post_likes WHERE user_id = ? AND post_id IN (`+db.Placeholders(len(postIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// sports returns {id, code, name} for ids (read-only on sports).
func (repo) sports(ctx context.Context, q db.Querier, ids []int64) (map[int64]SportRef, error) {
	out := map[int64]SportRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT id, code, name FROM sports WHERE id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s SportRef
		if err := rows.Scan(&s.ID, &s.Code, &s.Name); err != nil {
			return nil, err
		}
		out[s.ID] = s
	}
	return out, rows.Err()
}

// listComments returns visible comments of a post after afterID, oldest first, hiding
// comments by users blocked either way with viewer (2 viewer args).
func (repo) listComments(ctx context.Context, q db.Querier, viewer, postID, afterID int64, limit int) ([]commentRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.id, c.post_id, c.author_id, c.body, c.created_at FROM post_comments c
	  WHERE c.post_id = ? AND c.status = 'visible' AND c.id > ?
	  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = c.author_id) OR (b.blocker_id = c.author_id AND b.blocked_id = ?))
	  ORDER BY c.id ASC LIMIT ?`, postID, afterID, viewer, viewer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []commentRow{}
	for rows.Next() {
		var c commentRow
		if err := rows.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = c.CreatedAt.UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

func (repo) insertComment(ctx context.Context, q db.Querier, postID, author int64, body string) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO post_comments (post_id, author_id, body) VALUES (?, ?, ?)`, postID, author, body)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = q.ExecContext(ctx, `UPDATE posts SET comment_count = comment_count + 1 WHERE id = ?`, postID)
	return id, err
}

// deletableComment returns the post id of a visible comment that caller wrote or that sits
// on caller's post (sql.ErrNoRows otherwise).
func (repo) deletableComment(ctx context.Context, q db.Querier, commentID, caller int64) (int64, error) {
	var postID int64
	err := q.QueryRowContext(ctx, `SELECT c.post_id FROM post_comments c JOIN posts p ON p.id = c.post_id
	  WHERE c.id = ? AND c.status = 'visible' AND (c.author_id = ? OR p.author_id = ?) FOR UPDATE`, commentID, caller, caller).Scan(&postID)
	return postID, err
}

func (repo) deleteComment(ctx context.Context, q db.Querier, commentID, postID int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE post_comments SET status = 'deleted' WHERE id = ? AND status = 'visible'`, commentID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE posts SET comment_count = IF(comment_count > 0, comment_count - 1, 0) WHERE id = ?`, postID)
	return err
}
