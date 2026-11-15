package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

// repository holds every SQL statement of the module. Owned tables: notifications,
// device_tokens. Read-only: user_settings, users, profiles, user_roles, events,
// event_registrations, conversations, conversation_members, messages.
type repository struct{ db *sql.DB }

// ---- devices ----

func (r *repository) upsertDevice(ctx context.Context, userID int64, platform, token string, appVersion any) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO device_tokens (user_id, platform, token, app_version) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE user_id = VALUES(user_id), platform = VALUES(platform),
		   app_version = VALUES(app_version), last_seen_at = CURRENT_TIMESTAMP(3)`,
		userID, platform, token, appVersion)
	return err
}

func (r *repository) deleteDevice(ctx context.Context, userID int64, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM device_tokens WHERE token = ? AND user_id = ?`, token, userID)
	return err
}

// deleteUserDevices removes every push token of a user (account deletion).
func (r *repository) deleteUserDevices(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM device_tokens WHERE user_id = ?`, userID)
	return err
}

func (r *repository) deleteDeviceByID(ctx context.Context, id, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM device_tokens WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// devices returns up to maxTokensPerUser most recent tokens per user.
func (r *repository) devices(ctx context.Context, userIDs []int64) (map[int64][]device, error) {
	out := map[int64][]device{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, token FROM device_tokens WHERE user_id IN (`+db.Placeholders(len(userIDs))+`) ORDER BY user_id, id DESC`,
		db.Int64Args(userIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Token); err != nil {
			return nil, err
		}
		if len(out[d.UserID]) < maxTokensPerUser {
			out[d.UserID] = append(out[d.UserID], d)
		}
	}
	return out, rows.Err()
}

// ---- prefs ----

// prefs returns the effective notification prefs for each user (defaults when missing).
func (r *repository) prefs(ctx context.Context, userIDs []int64) (map[int64]map[string]bool, error) {
	out := map[int64]map[string]bool{}
	for _, id := range userIDs {
		out[id] = copyDefaults()
	}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, notification_prefs FROM user_settings WHERE user_id IN (`+db.Placeholders(len(userIDs))+`)`,
		db.Int64Args(userIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var p map[string]bool
		if json.Unmarshal(raw, &p) != nil {
			continue // malformed row: keep defaults
		}
		for k, v := range p {
			if _, known := defaultPrefs[k]; known {
				out[id][k] = v
			}
		}
	}
	return out, rows.Err()
}

func copyDefaults() map[string]bool {
	m := make(map[string]bool, len(defaultPrefs))
	for k, v := range defaultPrefs {
		m[k] = v
	}
	return m
}

// ---- notifications ----

func (r *repository) insert(ctx context.Context, userID int64, n note) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, type, title, body, data) VALUES (?, ?, ?, ?, ?)`,
		userID, n.Type, n.Title, n.Body, n.dataJSON())
	return err
}

func (r *repository) list(ctx context.Context, userID, beforeID int64, limit int) ([]Notification, error) {
	q := `SELECT id, type, title, body, data, read_at, created_at FROM notifications WHERE user_id = ?`
	args := []any{userID}
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
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var data []byte
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Body, &data, &readAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		if len(data) == 0 || !json.Valid(data) {
			data = []byte("{}")
		}
		n.Data = json.RawMessage(data)
		n.CreatedAt = n.CreatedAt.UTC()
		if readAt.Valid {
			t := readAt.Time.UTC()
			n.ReadAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *repository) unreadCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (r *repository) markRead(ctx context.Context, userID int64, ids []int64, now time.Time) error {
	args := append([]any{now, userID}, db.Int64Args(ids)...)
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL AND id IN (`+db.Placeholders(len(ids))+`)`,
		args...)
	return err
}

func (r *repository) markAllRead(ctx context.Context, userID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, now, userID)
	return err
}

// ---- read-only lookups for event texts and recipients ----

type eventInfo struct {
	Title       string
	Status      string
	OrganiserID int64
}

func (r *repository) event(ctx context.Context, id int64) (eventInfo, bool, error) {
	var e eventInfo
	err := r.db.QueryRowContext(ctx, `SELECT title, status, organiser_id FROM events WHERE id = ?`, id).Scan(&e.Title, &e.Status, &e.OrganiserID)
	if db.IsNoRows(err) {
		return e, false, nil
	}
	return e, err == nil, err
}

// eventPlayers lists users with a live registration (registered or waitlisted).
func (r *repository) eventPlayers(ctx context.Context, eventID int64) ([]int64, error) {
	return r.ids(ctx, `SELECT user_id FROM event_registrations WHERE event_id = ? AND status IN ('registered','waitlisted') ORDER BY id`, eventID)
}

type conversationInfo struct {
	Type   string
	Title  string
	Status string
}

func (r *repository) conversation(ctx context.Context, id int64) (conversationInfo, bool, error) {
	var c conversationInfo
	err := r.db.QueryRowContext(ctx, `SELECT type, COALESCE(title, ''), status FROM conversations WHERE id = ?`, id).Scan(&c.Type, &c.Title, &c.Status)
	if db.IsNoRows(err) {
		return c, false, nil
	}
	return c, err == nil, err
}

func (r *repository) message(ctx context.Context, id int64) (kind, status string, found bool, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT kind, status FROM messages WHERE id = ?`, id).Scan(&kind, &status)
	if db.IsNoRows(err) {
		return "", "", false, nil
	}
	return kind, status, err == nil, err
}

// unmutedMembers lists active, unmuted members of a conversation except one user.
func (r *repository) unmutedMembers(ctx context.Context, conversationID, except int64, now time.Time) ([]int64, error) {
	return r.ids(ctx,
		`SELECT user_id FROM conversation_members
		 WHERE conversation_id = ? AND status = 'active' AND user_id <> ? AND (muted_until IS NULL OR muted_until <= ?)
		 ORDER BY user_id`,
		conversationID, except, now)
}

func (r *repository) ids(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
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

// ---- broadcast ----

// insertBroadcast writes one row per matching user inside tx and returns the count and the
// id range written (ids may not be contiguous; the range is filtered by type later).
func (r *repository) insertBroadcast(ctx context.Context, tx db.Querier, in BroadcastInput) (n, first, last int64, err error) {
	var b strings.Builder
	b.WriteString(`INSERT INTO notifications (user_id, type, title, body, data)
		SELECT u.id, 'broadcast', ?, ?, JSON_OBJECT() FROM users u
		LEFT JOIN profiles p ON p.user_id = u.id
		WHERE u.status = 'active' AND u.onboarding_step = 'done'`)
	args := []any{in.Title, in.Body}
	if !in.Audience.IncludeMinors {
		b.WriteString(` AND u.is_minor = 0`)
	}
	if len(in.Audience.Roles) > 0 {
		b.WriteString(` AND EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role IN (` + db.Placeholders(len(in.Audience.Roles)) + `))`)
		for _, role := range in.Audience.Roles {
			args = append(args, role)
		}
	}
	if in.Audience.SportID > 0 {
		b.WriteString(` AND p.sport_id = ?`)
		args = append(args, in.Audience.SportID)
	}
	if in.Audience.State != "" {
		b.WriteString(` AND p.state = ?`)
		args = append(args, in.Audience.State)
	}
	res, err := tx.ExecContext(ctx, b.String(), args...)
	if err != nil {
		return 0, 0, 0, err
	}
	n, _ = res.RowsAffected()
	if n == 0 {
		return 0, 0, 0, nil
	}
	first, _ = res.LastInsertId()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM notifications WHERE type = 'broadcast' AND id >= ?`, first).Scan(&last); err != nil {
		return 0, 0, 0, err
	}
	return n, first, last, nil
}

func (r *repository) broadcastRows(ctx context.Context, afterID, lastID int64, limit int) ([]broadcastRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, title, body FROM notifications WHERE type = 'broadcast' AND id > ? AND id <= ? ORDER BY id LIMIT ?`,
		afterID, lastID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []broadcastRow
	for rows.Next() {
		var b broadcastRow
		if err := rows.Scan(&b.ID, &b.UserID, &b.Title, &b.Body); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---- reminders ----

func (r *repository) reminderRows(ctx context.Context, from, until time.Time, afterID int64, limit int) ([]reminderRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.id, r.event_id, r.user_id, e.title, e.starts_at
		 FROM event_registrations r JOIN events e ON e.id = r.event_id
		 WHERE r.status = 'registered' AND e.status = 'published' AND e.starts_at > ? AND e.starts_at <= ? AND r.id > ?
		 ORDER BY r.id LIMIT ?`,
		from, until, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []reminderRow
	for rows.Next() {
		var x reminderRow
		if err := rows.Scan(&x.RegID, &x.EventID, &x.UserID, &x.Title, &x.StartsAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
