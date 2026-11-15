package registration

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/eligibility"
	"athlix/backend/internal/platform/httpx"
)

const regCols = `r.id, r.event_id, r.user_id, r.team_name, r.team_members, r.status, r.payment_status,
	r.payment_note, r.created_at, r.qualified_via, r.qualified_identity_id, r.qualified_district_id,
	r.eligibility_flag, r.eligibility_flagged_at`

const evCols = `e.id, e.organiser_id, e.status, e.starts_at, e.registration_deadline, e.max_slots,
	e.registrations_count, e.gender_category, e.age_min, e.age_max, e.eligibility_type,
	e.eligibility_scope, e.eligibility_district_id, e.eligibility_state_id, e.require_verified`

type scanner interface{ Scan(dest ...any) error }

func regDest(r *rec, members *[]byte) []any {
	return []any{&r.ID, &r.EventID, &r.UserID, &r.TeamName, members, &r.Status, &r.PaymentStatus,
		&r.PaymentNote, &r.CreatedAt, &r.QualifiedVia, &r.QualifiedIdentityID, &r.QualifiedDistrictID,
		&r.EligibilityFlag, &r.EligibilityFlaggedAt}
}

func evDest(e *evRec) []any {
	return []any{&e.ID, &e.OrganiserID, &e.Status, &e.StartsAt, &e.Deadline, &e.MaxSlots,
		&e.RegistrationsCount, &e.GenderCategory, &e.AgeMin, &e.AgeMax, &e.ElType,
		&e.ElScope, &e.ElDistrictID, &e.ElStateID, &e.RequireVerified}
}

func finishRec(r *rec, members []byte) {
	r.TeamMembers = []TeamMember{}
	if len(members) > 0 {
		_ = json.Unmarshal(members, &r.TeamMembers)
	}
}

func scanReg(sc scanner) (rec, error) {
	var r rec
	var members []byte
	if err := sc.Scan(regDest(&r, &members)...); err != nil {
		return rec{}, err
	}
	finishRec(&r, members)
	return r, nil
}

// ruleOf is the stored location rule of an event.
func ruleOf(e evRec) eligibility.Rule {
	r := eligibility.Rule{Type: e.ElType, RequireVerified: e.RequireVerified}
	if e.ElScope != nil {
		r.Scope = *e.ElScope
	}
	if e.ElDistrictID != nil {
		r.DistrictID = *e.ElDistrictID
	}
	if e.ElStateID != nil {
		r.StateID = *e.ElStateID
	}
	return r
}

type repo struct{ db *sql.DB }

func (p *repo) q(q db.Querier) db.Querier {
	if q == nil {
		return p.db
	}
	return q
}

// event reads the event (events is owned by event; read-only here except registrations_count).
func (p *repo) event(ctx context.Context, q db.Querier, id int64, forUpdate bool) (evRec, error) {
	sq := `SELECT ` + evCols + ` FROM events e WHERE e.id = ?`
	if forUpdate {
		sq += ` FOR UPDATE`
	}
	var e evRec
	err := p.q(q).QueryRowContext(ctx, sq, id).Scan(evDest(&e)...)
	if db.IsNoRows(err) {
		return evRec{}, httpx.NotFound("event")
	}
	return e, err
}

// profileFacts returns the caller's birth year and verified gender (unknown = nil / "").
func (p *repo) profileFacts(ctx context.Context, q db.Querier, userID int64) (*int, string, error) {
	var by *int
	err := p.q(q).QueryRowContext(ctx, `SELECT birth_year FROM profiles WHERE user_id = ?`, userID).Scan(&by)
	if err != nil && !db.IsNoRows(err) {
		return nil, "", err
	}
	var g *string
	err = p.q(q).QueryRowContext(ctx, `SELECT gender FROM identity_verifications WHERE user_id = ? AND status = 'verified'
		ORDER BY id DESC LIMIT 1`, userID).Scan(&g)
	if err != nil && !db.IsNoRows(err) {
		return nil, "", err
	}
	gender := ""
	if g != nil {
		gender = *g
	}
	return by, gender, nil
}

// mine finds the caller's row on an event (nil when none). Locks it when forUpdate.
func (p *repo) mine(ctx context.Context, q db.Querier, eventID, userID int64, forUpdate bool) (*rec, error) {
	sq := `SELECT ` + regCols + ` FROM event_registrations r WHERE r.event_id = ? AND r.user_id = ?`
	if forUpdate {
		sq += ` FOR UPDATE`
	}
	r, err := scanReg(p.q(q).QueryRowContext(ctx, sq, eventID, userID))
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// onEvent loads one registration scoped to its event (404 when it is not on that event).
func (p *repo) onEvent(ctx context.Context, q db.Querier, eventID, regID int64, forUpdate bool) (rec, error) {
	sq := `SELECT ` + regCols + ` FROM event_registrations r WHERE r.id = ? AND r.event_id = ?`
	if forUpdate {
		sq += ` FOR UPDATE`
	}
	r, err := scanReg(p.q(q).QueryRowContext(ctx, sq, regID, eventID))
	if db.IsNoRows(err) {
		return rec{}, httpx.NotFound("registration")
	}
	return r, err
}

func (p *repo) insert(ctx context.Context, tx *sql.Tx, r rec) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO event_registrations (event_id, user_id, team_name, team_members,
		status, payment_status, qualified_via, qualified_identity_id, qualified_district_id)
		VALUES (?, ?, ?, ?, ?, 'unpaid', ?, ?, ?)`,
		r.EventID, r.UserID, r.TeamName, r.membersJSON(), r.Status, r.QualifiedVia, r.QualifiedIdentityID, r.QualifiedDistrictID)
	if err != nil {
		if db.IsDuplicate(err) {
			return 0, httpx.Conflict("You are already registered for this event.")
		}
		return 0, err
	}
	return res.LastInsertId()
}

// reactivate turns a withdrawn row into a new registration (UNIQUE(event_id, user_id)).
func (p *repo) reactivate(ctx context.Context, tx *sql.Tx, r rec) error {
	res, err := tx.ExecContext(ctx, `UPDATE event_registrations SET team_name = ?, team_members = ?, status = ?,
		payment_status = 'unpaid', payment_note = NULL, qualified_via = ?, qualified_identity_id = ?,
		qualified_district_id = ?, eligibility_flag = 'none', eligibility_flagged_at = NULL
		WHERE id = ? AND status = 'withdrawn'`,
		r.TeamName, r.membersJSON(), r.Status, r.QualifiedVia, r.QualifiedIdentityID, r.QualifiedDistrictID, r.ID)
	return one(res, err)
}

func one(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return httpx.Conflict("The registration changed; try again.")
	}
	return nil
}

// bump moves events.registrations_count atomically (the event row is already locked).
func (p *repo) bump(ctx context.Context, tx *sql.Tx, eventID int64, delta int) error {
	_, err := tx.ExecContext(ctx, `UPDATE events SET registrations_count = GREATEST(registrations_count + ?, 0) WHERE id = ?`, delta, eventID)
	return err
}

func (p *repo) setStatus(ctx context.Context, tx *sql.Tx, id int64, from, to string) error {
	res, err := tx.ExecContext(ctx, `UPDATE event_registrations SET status = ? WHERE id = ? AND status = ?`, to, id, from)
	return one(res, err)
}

func (p *repo) setPayment(ctx context.Context, tx *sql.Tx, id int64, status string, note *string) error {
	_, err := tx.ExecContext(ctx, `UPDATE event_registrations SET payment_status = ?, payment_note = ? WHERE id = ?`, status, note, id)
	return err
}

// waitlisted returns the oldest waitlisted rows of an event, locked.
func (p *repo) waitlisted(ctx context.Context, tx *sql.Tx, eventID int64, limit int) ([]rec, error) {
	return p.list(ctx, tx, `SELECT `+regCols+` FROM event_registrations r WHERE r.event_id = ? AND r.status = 'waitlisted'
		ORDER BY r.id ASC LIMIT ? FOR UPDATE`, eventID, limit)
}

// qualify stores a (re-)check result with the status and clears the eligibility flag.
func (p *repo) qualify(ctx context.Context, tx *sql.Tx, id int64, status string, res eligibility.Result) error {
	via := res.Via
	if via == "" {
		via = eligibility.TypeOpen
	}
	_, err := tx.ExecContext(ctx, `UPDATE event_registrations SET status = ?, qualified_via = ?, qualified_identity_id = ?,
		qualified_district_id = ?, eligibility_flag = 'none', eligibility_flagged_at = NULL WHERE id = ?`,
		status, via, db.NullInt64(res.IdentityID), db.NullInt64(res.DistrictID), id)
	return err
}

// flag sets the eligibility flag; false when it already had that flag (idempotent).
func (p *repo) flag(ctx context.Context, tx *sql.Tx, id int64, flag string, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE event_registrations SET eligibility_flag = ?, eligibility_flagged_at = ?
		WHERE id = ? AND eligibility_flag <> ?`, flag, now, id, flag)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// identityStatuses reads statuses of location identities (owned by locationidentity; read-only).
func (p *repo) identityStatuses(ctx context.Context, q db.Querier, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := p.q(q).QueryContext(ctx, `SELECT id, status FROM user_location_identities WHERE id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st string
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

func (p *repo) list(ctx context.Context, q db.Querier, sq string, args ...any) ([]rec, error) {
	rows, err := p.q(q).QueryContext(ctx, sq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rec
	for rows.Next() {
		r, err := scanReg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// byUser is GET /me/registrations (newest first, limit+1 rows).
func (p *repo) byUser(ctx context.Context, userID int64, status string, pg httpx.PageParams) ([]rec, error) {
	sq := `SELECT ` + regCols + ` FROM event_registrations r WHERE r.user_id = ?`
	args := []any{userID}
	if status != "" {
		sq += ` AND r.status = ?`
		args = append(args, status)
	}
	if pg.Cursor.ID > 0 {
		sq += ` AND r.id < ?`
		args = append(args, pg.Cursor.ID)
	}
	sq += ` ORDER BY r.id DESC LIMIT ?`
	return p.list(ctx, nil, sq, append(args, pg.Limit+1)...)
}

// byEvent is GET /events/:id/registrations (registration order, limit+1 rows).
func (p *repo) byEvent(ctx context.Context, eventID int64, status string, flaggedOnly bool, pg httpx.PageParams) ([]rec, error) {
	sq := `SELECT ` + regCols + ` FROM event_registrations r WHERE r.event_id = ?`
	args := []any{eventID}
	if status != "" {
		sq += ` AND r.status = ?`
		args = append(args, status)
	}
	if flaggedOnly {
		sq += ` AND r.eligibility_flag <> 'none'`
	}
	if pg.Cursor.ID > 0 {
		sq += ` AND r.id > ?`
		args = append(args, pg.Cursor.ID)
	}
	sq += ` ORDER BY r.id ASC LIMIT ?`
	return p.list(ctx, nil, sq, append(args, pg.Limit+1)...)
}

// byIdentity returns active registrations on events not yet started that qualified through
// identityID, locked, with their events. Rows are ordered by id after the cursor.
func (p *repo) byIdentity(ctx context.Context, tx *sql.Tx, userID, identityID, after int64, now time.Time, limit int) ([]rec, []evRec, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+regCols+`, `+evCols+` FROM event_registrations r
		JOIN events e ON e.id = r.event_id
		WHERE r.user_id = ? AND r.qualified_identity_id = ? AND r.status IN ('registered','waitlisted')
		AND e.status = 'published' AND e.starts_at > ? AND r.id > ?
		ORDER BY r.id ASC LIMIT ? FOR UPDATE`, userID, identityID, now, after, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var regs []rec
	var evs []evRec
	for rows.Next() {
		var r rec
		var e evRec
		var members []byte
		if err := rows.Scan(append(regDest(&r, &members), evDest(&e)...)...); err != nil {
			return nil, nil, err
		}
		finishRec(&r, members)
		regs = append(regs, r)
		evs = append(evs, e)
	}
	return regs, evs, rows.Err()
}
