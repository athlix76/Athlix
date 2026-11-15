package community

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

// repo is the SQL for conversations (type = 'community') and their conversation_members.
// Every value is a bind parameter.
type repo struct{}

// selectCommunity reads communities with the viewer's membership (first arg = viewer id).
const selectCommunity = `SELECT c.id, c.title, c.description, c.avatar_upload_id, c.owner_id, s.id, s.code, s.name,
	  c.city, c.state, c.visibility, c.join_policy, c.member_count, c.status, c.created_at, m.status, m.role,
	  c.removal_reason
	FROM conversations c
	LEFT JOIN sports s ON s.id = c.sport_id
	LEFT JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = ?
	WHERE c.type = 'community'`

// notBlockedByOwner hides communities whose owner and the viewer block each other (args: viewer, viewer).
const notBlockedByOwner = ` AND NOT EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = c.owner_id AND bl.blocked_id = ?) OR (bl.blocker_id = ? AND bl.blocked_id = c.owner_id))`

// visibleTo is the read rule: active, public or the viewer is a member / invited / pending,
// and no block with the owner (args: viewer, viewer).
const visibleTo = ` AND c.status = 'active' AND (c.visibility = 'public' OR m.status IN ('active','pending','invited'))` + notBlockedByOwner

type row struct {
	ID          int64
	Title       sql.NullString
	Description sql.NullString
	AvatarID    sql.NullInt64
	OwnerID     sql.NullInt64
	SportID     sql.NullInt64
	SportCode   sql.NullString
	SportName   sql.NullString
	City        sql.NullString
	State       sql.NullString
	Visibility  string
	JoinPolicy  string
	MemberCount int
	Status      string
	CreatedAt   time.Time
	MyStatus    sql.NullString
	MyRole      sql.NullString
	Removal     sql.NullString // conversations.removal_reason (admin list only)
}

func queryRows(ctx context.Context, q db.Querier, query string, args ...any) ([]row, error) {
	rs, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []row{}
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.ID, &r.Title, &r.Description, &r.AvatarID, &r.OwnerID, &r.SportID, &r.SportCode, &r.SportName,
			&r.City, &r.State, &r.Visibility, &r.JoinPolicy, &r.MemberCount, &r.Status, &r.CreatedAt, &r.MyStatus, &r.MyRole, &r.Removal); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// visible returns one community the viewer may see, or sql.ErrNoRows.
func (repo) visible(ctx context.Context, q db.Querier, viewer, id int64) (row, error) {
	rs, err := queryRows(ctx, q, selectCommunity+` AND c.id = ?`+visibleTo, viewer, id, viewer, viewer)
	if err != nil {
		return row{}, err
	}
	if len(rs) == 0 {
		return row{}, sql.ErrNoRows
	}
	return rs[0], nil
}

// likePattern escapes LIKE wildcards; the pattern is passed as a bind argument.
func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// browse lists public, active communities, newest first.
func (repo) browse(ctx context.Context, q db.Querier, viewer int64, f ListFilter, before int64, limit int) ([]row, error) {
	query := selectCommunity + ` AND c.visibility = 'public'` + visibleTo
	args := []any{viewer, viewer, viewer}
	if f.Q != "" {
		like := likePattern(f.Q)
		query += ` AND (c.title LIKE ? OR c.description LIKE ?)`
		args = append(args, like, like)
	}
	if f.SportID > 0 {
		query += ` AND c.sport_id = ?`
		args = append(args, f.SportID)
	}
	if f.State != "" {
		query += ` AND c.state = ?`
		args = append(args, f.State)
	}
	if before > 0 {
		query += ` AND c.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY c.id DESC LIMIT ?`
	args = append(args, limit)
	return queryRows(ctx, q, query, args...)
}

// mine lists active communities where the viewer is a member, invited or pending.
func (repo) mine(ctx context.Context, q db.Querier, viewer, before int64, limit int) ([]row, error) {
	query := selectCommunity + ` AND c.status = 'active' AND m.status IN ('active','pending','invited')`
	args := []any{viewer}
	if before > 0 {
		query += ` AND c.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY c.id DESC LIMIT ?`
	args = append(args, limit)
	return queryRows(ctx, q, query, args...)
}

// adminList lists communities of any status (viewer id 0 = no membership).
func (repo) adminList(ctx context.Context, q db.Querier, search, status string, before int64, limit int) ([]row, error) {
	query := selectCommunity
	args := []any{int64(0)}
	if status != "" {
		query += ` AND c.status = ?`
		args = append(args, status)
	}
	if search != "" {
		like := likePattern(search)
		query += ` AND (c.title LIKE ? OR c.description LIKE ?)`
		args = append(args, like, like)
	}
	if before > 0 {
		query += ` AND c.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY c.id DESC LIMIT ?`
	args = append(args, limit)
	return queryRows(ctx, q, query, args...)
}

// lockRow is a community locked for a membership write, with the caller's membership.
type lockRow struct {
	OwnerID     int64
	Title       string
	Visibility  string
	JoinPolicy  string
	MemberCount int
	Status      string
	MyStatus    string // "" = no row
	MyRole      string
	Blocked     bool // caller and owner block each other
}

// visible applies the read rule of visibleTo to a locked row.
func (l lockRow) visible() bool {
	if l.Blocked || l.Status != StatusActive {
		return false
	}
	switch l.MyStatus {
	case MemberActive, MemberPending, MemberInvited:
		return true
	}
	return l.Visibility == VisibilityPublic
}

// lock reads and locks the conversation row (FOR UPDATE OF c). Every membership change takes
// this lock first, so member_count stays consistent.
func (repo) lock(ctx context.Context, q db.Querier, id, caller int64) (lockRow, error) {
	var l lockRow
	err := q.QueryRowContext(ctx, `SELECT COALESCE(c.owner_id, 0), COALESCE(c.title, ''), c.visibility, c.join_policy, c.member_count, c.status,
	    COALESCE(m.status, ''), COALESCE(m.role, ''),
	    EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = c.owner_id AND bl.blocked_id = ?) OR (bl.blocker_id = ? AND bl.blocked_id = c.owner_id))
	  FROM conversations c
	  LEFT JOIN conversation_members m ON m.conversation_id = c.id AND m.user_id = ?
	  WHERE c.id = ? AND c.type = 'community' FOR UPDATE OF c`, caller, caller, caller, id).
		Scan(&l.OwnerID, &l.Title, &l.Visibility, &l.JoinPolicy, &l.MemberCount, &l.Status, &l.MyStatus, &l.MyRole, &l.Blocked)
	return l, err
}

// countOwned counts the active communities a user owns.
func (repo) countOwned(ctx context.Context, q db.Querier, userID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE type = 'community' AND owner_id = ? AND status = 'active'`, userID).Scan(&n)
	return n, err
}

// sportActive reports whether an active sport exists (read-only on sports).
func (repo) sportActive(ctx context.Context, q db.Querier, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sports WHERE id = ? AND active = 1`, id).Scan(&n)
	return n > 0, err
}

// fields are the editable columns of a community.
type fields struct {
	Title       string
	Description string
	AvatarID    int64
	SportID     int64
	City        string
	State       string
	Visibility  string
	JoinPolicy  string
}

func (repo) insert(ctx context.Context, q db.Querier, owner int64, f fields) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO conversations (type, title, description, avatar_upload_id, owner_id, sport_id, city, state, visibility, join_policy, member_count, status)
	  VALUES ('community', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'active')`,
		f.Title, db.NullString(f.Description), db.NullInt64(f.AvatarID), owner, db.NullInt64(f.SportID),
		db.NullString(f.City), db.NullString(f.State), f.Visibility, f.JoinPolicy)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, status) VALUES (?, ?, 'owner', 'active')`, id, owner)
	return id, err
}

func (repo) update(ctx context.Context, q db.Querier, id int64, f fields) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET title = ?, description = ?, avatar_upload_id = ?, sport_id = ?, city = ?, state = ?, visibility = ?, join_policy = ?
	  WHERE id = ? AND type = 'community' AND status = 'active'`,
		f.Title, db.NullString(f.Description), db.NullInt64(f.AvatarID), db.NullInt64(f.SportID),
		db.NullString(f.City), db.NullString(f.State), f.Visibility, f.JoinPolicy, id)
	return err
}

// current reads the editable columns for a partial update (inside the tx, after lock).
func (repo) current(ctx context.Context, q db.Querier, id int64) (fields, error) {
	var f fields
	var desc, city, state sql.NullString
	var avatar, sport sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT COALESCE(title, ''), description, avatar_upload_id, sport_id, city, state, visibility, join_policy FROM conversations WHERE id = ? AND type = 'community'`, id).
		Scan(&f.Title, &desc, &avatar, &sport, &city, &state, &f.Visibility, &f.JoinPolicy)
	f.Description, f.City, f.State = desc.String, city.String, state.String
	f.AvatarID, f.SportID = avatar.Int64, sport.Int64
	return f, err
}

func (repo) setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET status = ? WHERE id = ? AND type = 'community'`, status, id)
	return err
}

// remove sets status 'removed' and stores the admin's reason (conversations.removal_reason).
func (repo) remove(ctx context.Context, q db.Querier, id int64, reason string) error {
	_, err := q.ExecContext(ctx, `UPDATE conversations SET status = 'removed', removal_reason = ? WHERE id = ? AND type = 'community'`, reason, id)
	return err
}

// transferOwner moves ownership in one go: the conversation's owner_id, the new owner's role
// ('owner') and the old owner's role ('admin'). Both rows must be active; the caller holds the
// conversation row lock.
func (repo) transferOwner(ctx context.Context, q db.Querier, id, from, to int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE conversations SET owner_id = ? WHERE id = ? AND type = 'community' AND owner_id = ?`, to, id, from); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `UPDATE conversation_members SET role = 'owner' WHERE conversation_id = ? AND user_id = ? AND status = 'active'`, id, to); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE conversation_members SET role = 'admin' WHERE conversation_id = ? AND user_id = ? AND status = 'active'`, id, from)
	return err
}

// logTransfer writes the ownership audit row (community_ownership_transfers, migration 0012).
func (repo) logTransfer(ctx context.Context, q db.Querier, id, from, to int64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO community_ownership_transfers (conversation_id, from_user_id, to_user_id) VALUES (?, ?, ?)`, id, from, to)
	return err
}

// adjustCount adds +1 or -1 to member_count (never below zero).
func (repo) adjustCount(ctx context.Context, q db.Querier, id int64, delta int) error {
	query := `UPDATE conversations SET member_count = member_count + 1 WHERE id = ?`
	if delta < 0 {
		query = `UPDATE conversations SET member_count = GREATEST(member_count, 1) - 1 WHERE id = ?`
	}
	_, err := q.ExecContext(ctx, query, id)
	return err
}

// upsertMember sets a user's membership status (role member). Re-joining resets joined_at.
func (repo) upsertMember(ctx context.Context, q db.Querier, id, userID int64, status string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, status) VALUES (?, ?, 'member', ?)
	  ON DUPLICATE KEY UPDATE role = 'member', status = VALUES(status), joined_at = CURRENT_TIMESTAMP(3)`, id, userID, status)
	return err
}

// member reads one membership; found is false when there is no row.
func (repo) member(ctx context.Context, q db.Querier, id, userID int64) (status, role string, found bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT status, role FROM conversation_members WHERE conversation_id = ? AND user_id = ?`, id, userID).Scan(&status, &role)
	if db.IsNoRows(err) {
		return "", "", false, nil
	}
	return status, role, err == nil, err
}

// memberStatuses returns user id → status for the given users.
func (repo) memberStatuses(ctx context.Context, q db.Querier, id int64, userIDs []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	args := append([]any{id}, db.Int64Args(userIDs)...)
	rs, err := q.QueryContext(ctx, `SELECT user_id, status FROM conversation_members WHERE conversation_id = ? AND user_id IN (`+db.Placeholders(len(userIDs))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	for rs.Next() {
		var uid int64
		var st string
		if err := rs.Scan(&uid, &st); err != nil {
			return nil, err
		}
		out[uid] = st
	}
	return out, rs.Err()
}

// setMemberStatus changes a membership status; activating resets joined_at.
func (repo) setMemberStatus(ctx context.Context, q db.Querier, id, userID int64, status string) error {
	query := `UPDATE conversation_members SET status = ? WHERE conversation_id = ? AND user_id = ?`
	if status == MemberActive {
		query = `UPDATE conversation_members SET status = ?, joined_at = CURRENT_TIMESTAMP(3) WHERE conversation_id = ? AND user_id = ?`
	}
	_, err := q.ExecContext(ctx, query, status, id, userID)
	return err
}

// ban marks a user banned (also for non-members, so they cannot join later).
func (repo) ban(ctx context.Context, q db.Querier, id, userID int64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO conversation_members (conversation_id, user_id, role, status) VALUES (?, ?, 'member', 'banned')
	  ON DUPLICATE KEY UPDATE role = 'member', status = 'banned'`, id, userID)
	return err
}

func (repo) setRole(ctx context.Context, q db.Querier, id, userID int64, role string) error {
	_, err := q.ExecContext(ctx, `UPDATE conversation_members SET role = ? WHERE conversation_id = ? AND user_id = ? AND status = 'active'`, role, id, userID)
	return err
}

// deletePending removes a pending join request.
func (repo) deletePending(ctx context.Context, q db.Querier, id, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM conversation_members WHERE conversation_id = ? AND user_id = ? AND status = 'pending'`, id, userID)
	return err
}

type requestRow struct {
	UserID      int64
	RequestedAt time.Time
}

// joinRequests lists pending requests, oldest first, keyset on (joined_at, user_id).
func (repo) joinRequests(ctx context.Context, q db.Querier, id int64, afterTime time.Time, afterUser int64, limit int) ([]requestRow, error) {
	query := `SELECT user_id, joined_at FROM conversation_members WHERE conversation_id = ? AND status = 'pending'`
	args := []any{id}
	if afterUser > 0 {
		query += ` AND (joined_at > ? OR (joined_at = ? AND user_id > ?))`
		args = append(args, afterTime, afterTime, afterUser)
	}
	query += ` ORDER BY joined_at, user_id LIMIT ?`
	args = append(args, limit)
	rs, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []requestRow{}
	for rs.Next() {
		var r requestRow
		if err := rs.Scan(&r.UserID, &r.RequestedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}
