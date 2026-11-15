package chat

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
)

// All SQL of the chat module. Every value is a bind parameter.

const convSelect = `SELECT c.id, c.type, c.title, c.avatar_upload_id, c.event_id, c.member_count, c.last_message_id, c.status,
       COALESCE(c.last_message_at, c.created_at) AS activity_at, c.direct_key, m.role, m.muted_until,
       (SELECT COUNT(*) FROM messages x WHERE x.conversation_id = c.id AND x.id > COALESCE(m.last_read_message_id, 0)
          AND x.status = 'visible' AND (x.sender_id IS NULL OR x.sender_id <> m.user_id)) AS unread
FROM conversation_members m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.user_id = ? AND m.status = 'active' AND c.status <> 'removed'`

const msgCols = `x.id, x.conversation_id, x.sender_id, x.kind, x.body, x.media_upload_id, x.reply_to_id, x.client_msg_id, x.status, x.created_at`

func scanConvs(rows *sql.Rows) ([]convRow, error) {
	defer rows.Close()
	var out []convRow
	for rows.Next() {
		var r convRow
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.AvatarUploadID, &r.EventID, &r.MemberCount, &r.LastMessageID, &r.Status,
			&r.ActivityAt, &r.DirectKey, &r.Role, &r.MutedUntil, &r.Unread); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanMsgs(rows *sql.Rows) ([]msgRow, error) {
	defer rows.Close()
	var out []msgRow
	for rows.Next() {
		var r msgRow
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.SenderID, &r.Kind, &r.Body, &r.MediaUploadID, &r.ReplyToID,
			&r.ClientMsgID, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// listConversations pages the caller's conversations, latest activity first.
func listConversations(ctx context.Context, q db.Querier, userID int64, before time.Time, beforeID int64, limit int) ([]convRow, error) {
	query := convSelect
	args := []any{userID}
	if beforeID > 0 {
		query += ` AND (COALESCE(c.last_message_at, c.created_at) < ? OR (COALESCE(c.last_message_at, c.created_at) = ? AND c.id < ?))`
		args = append(args, before, before, beforeID)
	}
	query += ` ORDER BY activity_at DESC, c.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanConvs(rows)
}

// getConversation loads one conversation the caller is an active member of.
func getConversation(ctx context.Context, q db.Querier, userID, convID int64) (convRow, error) {
	rows, err := q.QueryContext(ctx, convSelect+` AND c.id = ?`, userID, convID)
	if err != nil {
		return convRow{}, err
	}
	rs, err := scanConvs(rows)
	if err != nil {
		return convRow{}, err
	}
	if len(rs) == 0 {
		return convRow{}, sql.ErrNoRows
	}
	return rs[0], nil
}

// getMembership returns the caller's active membership; sql.ErrNoRows if not a member.
func getMembership(ctx context.Context, q db.Querier, convID, userID int64) (memberRow, error) {
	var r memberRow
	err := q.QueryRowContext(ctx, `SELECT c.id, c.type, c.status, c.direct_key, m.role
FROM conversations c JOIN conversation_members m ON m.conversation_id = c.id
WHERE c.id = ? AND m.user_id = ? AND m.status = 'active' AND c.status <> 'removed'`, convID, userID).
		Scan(&r.ConversationID, &r.Type, &r.Status, &r.DirectKey, &r.Role)
	return r, err
}

// eventConversationID returns the event group the caller is an active member of.
func eventConversationID(ctx context.Context, q db.Querier, userID, eventID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT c.id FROM conversations c
JOIN conversation_members m ON m.conversation_id = c.id
WHERE c.event_id = ? AND c.type = 'event' AND m.user_id = ? AND m.status = 'active' AND c.status <> 'removed'`, eventID, userID).Scan(&id)
	return id, err
}

// messagesPage returns messages newest first (id < beforeID when beforeID > 0).
func messagesPage(ctx context.Context, q db.Querier, convID, beforeID int64, limit int) ([]msgRow, error) {
	query := `SELECT ` + msgCols + ` FROM messages x WHERE x.conversation_id = ?`
	args := []any{convID}
	if beforeID > 0 {
		query += ` AND x.id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY x.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanMsgs(rows)
}

// messagesByID loads messages by id (any conversation; callers scope the ids).
func messagesByID(ctx context.Context, q db.Querier, ids []int64) ([]msgRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+msgCols+` FROM messages x WHERE x.id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	return scanMsgs(rows)
}

// messageByClientID finds the sender's message with this clientMsgId.
func messageByClientID(ctx context.Context, q db.Querier, senderID int64, clientMsgID string) (msgRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+msgCols+` FROM messages x WHERE x.sender_id = ? AND x.client_msg_id = ?`, senderID, clientMsgID)
	if err != nil {
		return msgRow{}, err
	}
	rs, err := scanMsgs(rows)
	if err != nil {
		return msgRow{}, err
	}
	if len(rs) == 0 {
		return msgRow{}, sql.ErrNoRows
	}
	return rs[0], nil
}

// messageInConversation reports whether messageID belongs to convID.
func messageInConversation(ctx context.Context, q db.Querier, messageID, convID int64) (bool, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM messages WHERE id = ? AND conversation_id = ?`, messageID, convID).Scan(&id)
	if db.IsNoRows(err) {
		return false, nil
	}
	return err == nil, err
}

func insertMessage(ctx context.Context, q db.Querier, r msgRow) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO messages (conversation_id, sender_id, kind, body, media_upload_id, reply_to_id, client_msg_id, status, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'visible', ?)`,
		r.ConversationID, r.SenderID.Int64, r.Kind, nullStr(r.Body), nullInt(r.MediaUploadID), nullInt(r.ReplyToID), nullStr(r.ClientMsgID), r.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func touchConversation(ctx context.Context, q db.Querier, convID, messageID int64, at time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET last_message_id = ?, last_message_at = ? WHERE id = ?`, messageID, at, convID)
	return err
}

// markRead moves last_read_message_id forward only (never back).
func markRead(ctx context.Context, q db.Querier, convID, userID, messageID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE conversation_members SET last_read_message_id = GREATEST(COALESCE(last_read_message_id, 0), ?)
WHERE conversation_id = ? AND user_id = ? AND status = 'active'`, messageID, convID, userID)
	return err
}

func setMute(ctx context.Context, q db.Querier, convID, userID int64, until *time.Time) error {
	var v any
	if until != nil {
		v = until.UTC()
	}
	_, err := q.ExecContext(ctx, `UPDATE conversation_members SET muted_until = ? WHERE conversation_id = ? AND user_id = ? AND status = 'active'`, v, convID, userID)
	return err
}

// setMemberLeft marks an active member as left (never the owner).
func setMemberLeft(ctx context.Context, q db.Querier, convID, userID int64) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE conversation_members SET status = 'left' WHERE conversation_id = ? AND user_id = ? AND status = 'active' AND role <> 'owner'`, convID, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// recountMembers sets member_count from the active rows (idempotent).
func recountMembers(ctx context.Context, q db.Querier, convID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET member_count = (SELECT COUNT(*) FROM conversation_members WHERE conversation_id = ? AND status = 'active') WHERE id = ?`, convID, convID)
	return err
}

// activeMemberIDs lists the user ids of active members.
func activeMemberIDs(ctx context.Context, q db.Querier, convID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id FROM conversation_members WHERE conversation_id = ? AND status = 'active'`, convID)
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

type memberListRow struct {
	UserID   int64
	Role     string
	JoinedAt time.Time
}

// listMembers pages active members by user id.
func listMembers(ctx context.Context, q db.Querier, convID, afterUserID int64, limit int) ([]memberListRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT user_id, role, joined_at FROM conversation_members
WHERE conversation_id = ? AND status = 'active' AND user_id > ? ORDER BY user_id LIMIT ?`, convID, afterUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memberListRow
	for rows.Next() {
		var r memberListRow
		if err := rows.Scan(&r.UserID, &r.Role, &r.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// deletableMessage loads a message with the caller's role in its conversation.
type deletableRow struct {
	ConversationID int64
	SenderID       sql.NullInt64
	Status         string
	ConvType       string
	Role           string
}

func getDeletable(ctx context.Context, q db.Querier, messageID, userID int64) (deletableRow, error) {
	var r deletableRow
	err := q.QueryRowContext(ctx, `SELECT x.conversation_id, x.sender_id, x.status, c.type, m.role
FROM messages x
JOIN conversations c ON c.id = x.conversation_id
JOIN conversation_members m ON m.conversation_id = x.conversation_id AND m.user_id = ? AND m.status = 'active'
WHERE x.id = ? AND c.status <> 'removed'`, userID, messageID).Scan(&r.ConversationID, &r.SenderID, &r.Status, &r.ConvType, &r.Role)
	return r, err
}

func softDeleteMessage(ctx context.Context, q db.Querier, messageID int64) (bool, error) {
	res, err := q.ExecContext(ctx, `UPDATE messages SET status = 'deleted' WHERE id = ? AND status = 'visible'`, messageID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// directConversationID finds a direct conversation by its key.
func directConversationID(ctx context.Context, q db.Querier, key string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM conversations WHERE direct_key = ? AND type = 'direct'`, key).Scan(&id)
	return id, err
}

func insertDirect(ctx context.Context, q db.Querier, key string, a, b int64) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO conversations (type, direct_key, visibility, join_policy, member_count) VALUES ('direct', ?, 'private', 'invite', 2)`, key)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, status) VALUES (?, ?, 'member', 'active'), (?, ?, 'member', 'active')`, id, a, id, b)
	return id, err
}

// openReportExists reports whether a conversation, or any of its messages, has an open report.
func openReportExists(ctx context.Context, q db.Querier, convID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports r
WHERE r.status = 'open' AND ((r.target_type = 'conversation' AND r.target_id = ?)
   OR (r.target_type = 'message' AND r.target_id IN (SELECT id FROM messages WHERE conversation_id = ?)))`, convID, convID).Scan(&n)
	return n > 0, err
}

// ---- user facts for the DM rules (read-only tables owned by user / consultation) ----

// allowDmsFrom reads user_settings.privacy.allowDmsFrom ("" when unset).
func allowDmsFrom(ctx context.Context, q db.Querier, userID int64) (string, error) {
	var v sql.NullString
	err := q.QueryRowContext(ctx, `SELECT JSON_UNQUOTE(JSON_EXTRACT(privacy, '$.allowDmsFrom')) FROM user_settings WHERE user_id = ?`, userID).Scan(&v)
	if db.IsNoRows(err) {
		return "", nil
	}
	return v.String, err
}

func acceptedConsultation(ctx context.Context, q db.Querier, a, b int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM consultation_requests WHERE status = 'accepted'
AND ((coach_id = ? AND player_id = ?) OR (coach_id = ? AND player_id = ?))`, a, b, b, a).Scan(&n)
	return n > 0, err
}

// ---- event groups ----

type eventRow struct {
	OrganiserID int64
	Title       string
	SportID     int64
	City        string
	State       string
	Status      string
}

func getEvent(ctx context.Context, q db.Querier, eventID int64) (eventRow, error) {
	var r eventRow
	err := q.QueryRowContext(ctx, `SELECT organiser_id, title, sport_id, city, state, status FROM events WHERE id = ?`, eventID).
		Scan(&r.OrganiserID, &r.Title, &r.SportID, &r.City, &r.State, &r.Status)
	return r, err
}

// upsertEventGroup creates the event group once (event_id is UNIQUE) and returns its id.
func upsertEventGroup(ctx context.Context, q db.Querier, eventID int64, e eventRow) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO conversations (type, title, owner_id, event_id, sport_id, city, state, visibility, join_policy, member_count)
VALUES ('event', ?, ?, ?, ?, ?, ?, 'private', 'invite', 0)
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		truncateRunes(e.Title, eventTitleRunes), e.OrganiserID, eventID, e.SportID, e.City, e.State)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type eventGroupMember struct {
	ConversationID int64
	ConvStatus     string
	MemberStatus   sql.NullString
	MemberCount    int
}

// lockEventGroup locks the event group row and returns the user's membership status.
func lockEventGroup(ctx context.Context, q db.Querier, eventID, userID int64) (eventGroupMember, error) {
	var r eventGroupMember
	err := q.QueryRowContext(ctx, `SELECT c.id, c.status, c.member_count, m.status
FROM conversations c LEFT JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = ?
WHERE c.event_id = ? AND c.type = 'event' FOR UPDATE`, userID, eventID).Scan(&r.ConversationID, &r.ConvStatus, &r.MemberCount, &r.MemberStatus)
	return r, err
}

// addMember inserts or re-activates a member. Banned members stay banned.
func addMember(ctx context.Context, q db.Querier, convID, userID int64, role string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, status) VALUES (?, ?, ?, 'active')
ON DUPLICATE KEY UPDATE status = IF(status = 'banned', status, 'active')`, convID, userID, role)
	return err
}

// hideUserMessages marks every visible message a user sent as deleted (account deletion).
// Uses uq_messages_client (sender_id, ...).
func hideUserMessages(ctx context.Context, q db.Querier, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE messages SET status = 'deleted' WHERE sender_id = ? AND status = 'visible'`, userID)
	return err
}

func archiveEventGroup(ctx context.Context, q db.Querier, eventID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET status = 'archived' WHERE event_id = ? AND type = 'event' AND status = 'active'`, eventID)
	return err
}

func nullStr(ns sql.NullString) any {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

func nullInt(ni sql.NullInt64) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}

// unreadTotals counts unread messages across the caller's active, unmuted conversations
// (same unread rule as convSelect).
func unreadTotals(ctx context.Context, q db.Querier, userID int64, now time.Time) (conversations, messages int, err error) {
	err = q.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(t.unread), 0) FROM (
  SELECT (SELECT COUNT(*) FROM messages x WHERE x.conversation_id = c.id AND x.id > COALESCE(m.last_read_message_id, 0)
            AND x.status = 'visible' AND (x.sender_id IS NULL OR x.sender_id <> m.user_id)) AS unread
  FROM conversation_members m
  JOIN conversations c ON c.id = m.conversation_id
  WHERE m.user_id = ? AND m.status = 'active' AND c.status <> 'removed' AND (m.muted_until IS NULL OR m.muted_until <= ?)
) t WHERE t.unread > 0`, userID, now).Scan(&conversations, &messages)
	return
}
