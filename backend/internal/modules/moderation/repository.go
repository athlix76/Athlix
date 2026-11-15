package moderation

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

type repo struct{ db *sql.DB }

// Per-target SQL. Table names are fixed strings chosen from this allowlist; never from input.

// visibleSQL answers "may the viewer see this target?" (COUNT > 0). Args: viewer..., id.
var visibleSQL = map[string]string{
	TargetUser: `SELECT COUNT(*) FROM users WHERE id = ? AND status = 'active'`,
	TargetEvent: `SELECT COUNT(*) FROM events WHERE id = ?
	  AND (status IN ('published','completed','cancelled') OR organiser_id = ?)`,
	TargetPost: `SELECT COUNT(*) FROM posts p WHERE p.id = ? AND p.status = 'visible'
	  AND (p.author_id = ? OR p.visibility = 'public'
	       OR EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = ? AND f.followee_id = p.author_id))
	  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = p.author_id) OR (b.blocker_id = p.author_id AND b.blocked_id = ?))`,
	TargetComment: `SELECT COUNT(*) FROM post_comments c JOIN posts p ON p.id = c.post_id
	  WHERE c.id = ? AND c.status = 'visible' AND p.status = 'visible'
	  AND (p.author_id = ? OR p.visibility = 'public'
	       OR EXISTS (SELECT 1 FROM follows f WHERE f.follower_id = ? AND f.followee_id = p.author_id))
	  AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = ? AND b.blocked_id = c.author_id) OR (b.blocker_id = c.author_id AND b.blocked_id = ?))`,
	TargetMessage: `SELECT COUNT(*) FROM messages m
	  JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.status = 'active'
	  WHERE m.id = ? AND m.status = 'visible' AND cm.user_id = ?`,
	TargetConversation: `SELECT COUNT(*) FROM conversations c
	  LEFT JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = ?
	  WHERE c.id = ? AND c.status = 'active'
	  AND (cm.status = 'active' OR (c.type = 'community' AND c.visibility = 'public'))`,
	TargetCommunity: `SELECT COUNT(*) FROM conversations c
	  LEFT JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = ?
	  WHERE c.id = ? AND c.type = 'community' AND c.status = 'active'
	  AND (c.visibility = 'public' OR cm.status IN ('active', 'invited'))`,
}

// visibleArgs orders the bind args for visibleSQL.
func visibleArgs(targetType string, viewer, id int64) []any {
	switch targetType {
	case TargetUser:
		return []any{id}
	case TargetEvent, TargetMessage:
		return []any{id, viewer}
	case TargetPost, TargetComment:
		return []any{id, viewer, viewer, viewer, viewer}
	default: // conversation, community
		return []any{viewer, id}
	}
}

// lockSQL reads status + owner (+ sport for events) FOR UPDATE.
var lockSQL = map[string]string{
	TargetUser:         `SELECT status, id, 0 FROM users WHERE id = ? FOR UPDATE`,
	TargetEvent:        `SELECT status, organiser_id, sport_id FROM events WHERE id = ? FOR UPDATE`,
	TargetPost:         `SELECT status, author_id, 0 FROM posts WHERE id = ? FOR UPDATE`,
	TargetComment:      `SELECT status, author_id, 0 FROM post_comments WHERE id = ? FOR UPDATE`,
	TargetMessage:      `SELECT status, COALESCE(sender_id, 0), 0 FROM messages WHERE id = ? FOR UPDATE`,
	TargetConversation: `SELECT status, COALESCE(owner_id, 0), 0 FROM conversations WHERE id = ? FOR UPDATE`,
	TargetCommunity:    `SELECT status, COALESCE(owner_id, 0), 0 FROM conversations WHERE id = ? AND type = 'community' FOR UPDATE`,
}

// statusSQL updates ONLY the status column (docs/architecture.md exception 1).
var statusSQL = map[string]string{
	TargetUser:         `UPDATE users SET status = ? WHERE id = ?`,
	TargetEvent:        `UPDATE events SET status = ? WHERE id = ?`,
	TargetPost:         `UPDATE posts SET status = ? WHERE id = ?`,
	TargetComment:      `UPDATE post_comments SET status = ? WHERE id = ?`,
	TargetMessage:      `UPDATE messages SET status = ? WHERE id = ?`,
	TargetConversation: `UPDATE conversations SET status = ? WHERE id = ?`,
	TargetCommunity:    `UPDATE conversations SET status = ? WHERE id = ? AND type = 'community'`,
}

// autoHideSQL hides a visible item; 0 rows affected = already hidden/removed.
var autoHideSQL = map[string]string{
	TargetPost:    `UPDATE posts SET status = 'hidden' WHERE id = ? AND status = 'visible'`,
	TargetComment: `UPDATE post_comments SET status = 'hidden' WHERE id = ? AND status = 'visible'`,
	TargetMessage: `UPDATE messages SET status = 'hidden' WHERE id = ? AND status = 'visible'`,
}

// previewSQL returns (id, title, text, owner_id, conversation_id) for an IN list.
var previewSQL = map[string]string{
	TargetUser:         `SELECT u.id, u.display_name, p.bio, u.id, NULL FROM users u LEFT JOIN profiles p ON p.user_id = u.id WHERE u.id IN `,
	TargetEvent:        `SELECT id, title, description, organiser_id, NULL FROM events WHERE id IN `,
	TargetPost:         `SELECT id, NULL, body, author_id, NULL FROM posts WHERE id IN `,
	TargetComment:      `SELECT id, NULL, body, author_id, NULL FROM post_comments WHERE id IN `,
	TargetMessage:      `SELECT id, NULL, body, sender_id, conversation_id FROM messages WHERE id IN `,
	TargetConversation: `SELECT id, title, description, owner_id, id FROM conversations WHERE id IN `,
	TargetCommunity:    `SELECT id, title, description, owner_id, id FROM conversations WHERE type = 'community' AND id IN `,
}

func (r *repo) targetVisible(ctx context.Context, targetType string, viewer, id int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, visibleSQL[targetType], visibleArgs(targetType, viewer, id)...).Scan(&n)
	return n > 0, err
}

func (r *repo) lockTarget(ctx context.Context, q db.Querier, targetType string, id int64) (targetState, error) {
	var s targetState
	err := q.QueryRowContext(ctx, lockSQL[targetType], id).Scan(&s.Status, &s.OwnerID, &s.SportID)
	return s, err
}

func (r *repo) setTargetStatus(ctx context.Context, q db.Querier, targetType string, id int64, status string) error {
	_, err := q.ExecContext(ctx, statusSQL[targetType], status, id)
	return err
}

// autoHide hides the target if it is still visible; reports whether it changed.
func (r *repo) autoHide(ctx context.Context, q db.Querier, targetType string, id int64) (bool, error) {
	res, err := q.ExecContext(ctx, autoHideSQL[targetType], id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *repo) findReport(ctx context.Context, q db.Querier, reporter int64, targetType string, targetID int64) (int64, bool, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM reports WHERE reporter_id = ? AND target_type = ? AND target_id = ?`,
		reporter, targetType, targetID).Scan(&id)
	if db.IsNoRows(err) {
		return 0, false, nil
	}
	return id, err == nil, err
}

func (r *repo) insertReport(ctx context.Context, q db.Querier, reporter int64, in CreateReportInput) (int64, error) {
	var details any
	if in.Details != nil {
		details = *in.Details
	}
	res, err := q.ExecContext(ctx, `INSERT INTO reports (reporter_id, target_type, target_id, reason, details) VALUES (?, ?, ?, ?, ?)`,
		reporter, in.TargetType, in.TargetID, in.Reason, details)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// countOpen counts open reports on a target. uq_reports_once makes each one a distinct reporter.
func (r *repo) countOpen(ctx context.Context, q db.Querier, targetType string, targetID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports WHERE target_type = ? AND target_id = ? AND status = 'open'`,
		targetType, targetID).Scan(&n)
	return n, err
}

func (r *repo) insertAction(ctx context.Context, q db.Querier, adminID int64, targetType string, targetID int64, action, reason string, reportID int64) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO moderation_actions (admin_id, target_type, target_id, action, reason, report_id) VALUES (?, ?, ?, ?, ?, ?)`,
		db.NullInt64(adminID), targetType, targetID, action, db.NullString(reason), db.NullInt64(reportID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// lastActionIsAutoHide reports whether the newest action on the target is the automatic hide.
func (r *repo) lastActionIsAutoHide(ctx context.Context, q db.Querier, targetType string, targetID int64) (bool, error) {
	var action string
	err := q.QueryRowContext(ctx, `SELECT action FROM moderation_actions WHERE target_type = ? AND target_id = ? ORDER BY id DESC LIMIT 1`,
		targetType, targetID).Scan(&action)
	if db.IsNoRows(err) {
		return false, nil
	}
	return action == ActionAutoHide, err
}

const reportCols = `SELECT r.id, r.reporter_id, r.target_type, r.target_id, r.reason, r.details, r.status,
	r.resolution_note, r.created_at, r.resolved_at,
	(SELECT COUNT(*) FROM reports o WHERE o.target_type = r.target_type AND o.target_id = r.target_id AND o.status = 'open')
	FROM reports r`

func scanReport(sc interface{ Scan(...any) error }) (reportRow, error) {
	var x reportRow
	var details, note sql.NullString
	var resolved sql.NullTime
	if err := sc.Scan(&x.ID, &x.ReporterID, &x.TargetType, &x.TargetID, &x.Reason, &details, &x.Status,
		&note, &x.CreatedAt, &resolved, &x.OpenOnTarget); err != nil {
		return x, err
	}
	if details.Valid {
		x.Details = &details.String
	}
	if note.Valid {
		x.ResolutionNote = &note.String
	}
	if resolved.Valid {
		t := resolved.Time
		x.ResolvedAt = &t
	}
	return x, nil
}

func (r *repo) getReport(ctx context.Context, q db.Querier, id int64, forUpdate bool) (reportRow, error) {
	s := reportCols + ` WHERE r.id = ?`
	if forUpdate {
		s += ` FOR UPDATE`
	}
	return scanReport(q.QueryRowContext(ctx, s, id))
}

// listReports orders underage_safety first, then newest. Cursor.Time is the bucket ("u" or "o").
func (r *repo) listReports(ctx context.Context, f ReportFilter, p httpx.PageParams) ([]reportRow, error) {
	s := reportCols + ` WHERE 1 = 1`
	var args []any
	if f.Status != "" {
		s += ` AND r.status = ?`
		args = append(args, f.Status)
	}
	if f.TargetType != "" {
		s += ` AND r.target_type = ?`
		args = append(args, f.TargetType)
	}
	if f.Reason != "" {
		s += ` AND r.reason = ?`
		args = append(args, f.Reason)
	}
	if p.Cursor.ID > 0 {
		if p.Cursor.Time == "u" {
			s += ` AND ((r.reason = 'underage_safety' AND r.id < ?) OR r.reason <> 'underage_safety')`
		} else {
			s += ` AND r.reason <> 'underage_safety' AND r.id < ?`
		}
		args = append(args, p.Cursor.ID)
	}
	s += ` ORDER BY (r.reason = 'underage_safety') DESC, r.id DESC LIMIT ?`
	args = append(args, p.Limit+1)
	rows, err := r.db.QueryContext(ctx, s, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []reportRow{}
	for rows.Next() {
		x, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *repo) setReportStatus(ctx context.Context, q db.Querier, id int64, status string, adminID int64, note string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE reports SET status = ?, resolved_by = ?, resolution_note = ?, resolved_at = ? WHERE id = ? AND status = 'open'`,
		status, adminID, db.NullString(note), now, id)
	return err
}

// actionOpenReports marks every open report on the target as actioned.
func (r *repo) actionOpenReports(ctx context.Context, q db.Querier, targetType string, targetID, adminID int64, note string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE reports SET status = 'actioned', resolved_by = ?, resolution_note = ?, resolved_at = ? WHERE target_type = ? AND target_id = ? AND status = 'open'`,
		adminID, db.NullString(note), now, targetType, targetID)
	return err
}

// previews returns (title, text, ownerId) per target id of one type.
func (r *repo) previews(ctx context.Context, targetType string, ids []int64) (map[int64]TargetPreview, error) {
	out := map[int64]TargetPreview{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, previewSQL[targetType]+`(`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title, text sql.NullString
		var owner, conv sql.NullInt64
		if err := rows.Scan(&id, &title, &text, &owner, &conv); err != nil {
			return nil, err
		}
		var p TargetPreview
		if title.Valid {
			t := title.String
			p.Title = &t
		}
		if text.Valid {
			t := truncate(text.String, previewLen)
			p.Text = &t
		}
		if owner.Valid {
			o := owner.Int64
			p.OwnerID = &o
		}
		if conv.Valid {
			v := conv.Int64
			p.ConversationID = &v
		}
		out[id] = p
	}
	return out, rows.Err()
}

const actionCols = `SELECT a.id, a.admin_id, au.name, a.target_type, a.target_id, a.action, a.reason, a.report_id, a.created_at
	FROM moderation_actions a LEFT JOIN admin_users au ON au.id = a.admin_id`

func scanAction(sc interface{ Scan(...any) error }) (actionRow, error) {
	var x actionRow
	var adminID, reportID sql.NullInt64
	var name, reason sql.NullString
	if err := sc.Scan(&x.ID, &adminID, &name, &x.TargetType, &x.TargetID, &x.Action, &reason, &reportID, &x.CreatedAt); err != nil {
		return x, err
	}
	if adminID.Valid {
		v := adminID.Int64
		x.AdminID = &v
	}
	if name.Valid {
		x.AdminName = &name.String
	}
	if reason.Valid {
		x.Reason = &reason.String
	}
	if reportID.Valid {
		v := reportID.Int64
		x.ReportID = &v
	}
	return x, nil
}

func (r *repo) getAction(ctx context.Context, id int64) (actionRow, error) {
	return scanAction(r.db.QueryRowContext(ctx, actionCols+` WHERE a.id = ?`, id))
}

func (r *repo) listActions(ctx context.Context, targetType string, targetID int64, p httpx.PageParams) ([]actionRow, error) {
	s := actionCols + ` WHERE 1 = 1`
	var args []any
	if targetType != "" {
		s += ` AND a.target_type = ?`
		args = append(args, targetType)
	}
	if targetID > 0 {
		s += ` AND a.target_id = ?`
		args = append(args, targetID)
	}
	if p.Cursor.ID > 0 {
		s += ` AND a.id < ?`
		args = append(args, p.Cursor.ID)
	}
	s += ` ORDER BY a.id DESC LIMIT ?`
	args = append(args, p.Limit+1)
	rows, err := r.db.QueryContext(ctx, s, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []actionRow{}
	for rows.Next() {
		x, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
