package event

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

// eventCols is the SELECT list for one events row joined with sports (alias e, s).
const eventCols = `e.id, e.organiser_id, e.sport_id, s.code, s.name, e.title, e.description, e.event_type,
	e.participant_type, e.team_size_min, e.team_size_max, e.competition_level, e.category,
	e.eligibility_type, e.eligibility_scope, e.eligibility_district_id, e.eligibility_state_id,
	e.require_verified, e.gender_category, e.age_min, e.age_max, e.venue_name, e.address_line,
	e.city, e.district, e.state, e.pincode, e.latitude, e.longitude, e.starts_at, e.ends_at,
	e.registration_deadline, e.entry_fee_paise, e.payment_modes, e.payment_instructions,
	e.max_slots, e.rules, e.prize_info, e.contact_name, e.contact_phone_last4, e.banner_upload_id,
	e.status, e.rejection_reason, e.cancel_reason, e.registrations_count, e.featured_at,
	e.published_at, e.created_at`

const eventFrom = ` FROM events e JOIN sports s ON s.id = e.sport_id`

// haversine is the great-circle distance in km between (?, ?) and the event venue.
// Args: lat, lat, lng (bound; device coordinates are never stored or logged).
const haversine = `(12742 * ASIN(SQRT(POWER(SIN(RADIANS(e.latitude - ?) / 2), 2) +
	COS(RADIANS(?)) * COS(RADIANS(e.latitude)) * POWER(SIN(RADIANS(e.longitude - ?) / 2), 2))))`

// notBlocked excludes organisers blocked either way with the viewer. Args: viewer, viewer.
const notBlocked = ` NOT EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = ? AND bl.blocked_id = e.organiser_id)
	OR (bl.blocker_id = e.organiser_id AND bl.blocked_id = ?))`

type scanner interface{ Scan(dest ...any) error }

func scanRec(sc scanner, withDist bool) (rec, error) {
	var r rec
	var modes []byte
	dest := []any{&r.ID, &r.OrganiserID, &r.SportID, &r.SportCode, &r.SportName, &r.Title, &r.Description,
		&r.EventType, &r.ParticipantType, &r.TeamSizeMin, &r.TeamSizeMax, &r.CompetitionLevel, &r.Category,
		&r.ElType, &r.ElScope, &r.ElDistrictID, &r.ElStateID, &r.RequireVerified, &r.GenderCategory,
		&r.AgeMin, &r.AgeMax, &r.VenueName, &r.AddressLine, &r.City, &r.District, &r.State, &r.Pincode,
		&r.Latitude, &r.Longitude, &r.StartsAt, &r.EndsAt, &r.Deadline, &r.EntryFeePaise, &modes,
		&r.PaymentInstructions, &r.MaxSlots, &r.Rules, &r.PrizeInfo, &r.ContactName, &r.PhoneLast4,
		&r.BannerUploadID, &r.Status, &r.RejectionReason, &r.CancelReason, &r.RegistrationsCount,
		&r.FeaturedAt, &r.PublishedAt, &r.CreatedAt}
	if withDist {
		dest = append(dest, &r.dist)
	}
	if err := sc.Scan(dest...); err != nil {
		return rec{}, err
	}
	r.PaymentModes = []string{}
	if len(modes) > 0 {
		_ = json.Unmarshal(modes, &r.PaymentModes)
	}
	return r, nil
}

type repo struct{ db *sql.DB }

func (p *repo) q(q db.Querier) db.Querier {
	if q == nil {
		return p.db
	}
	return q
}

// get loads one event; forUpdate locks the row (pass a tx).
func (p *repo) get(ctx context.Context, q db.Querier, id int64, forUpdate bool) (rec, error) {
	sq := `SELECT ` + eventCols + eventFrom + ` WHERE e.id = ?`
	if forUpdate {
		sq += ` FOR UPDATE`
	}
	r, err := scanRec(p.q(q).QueryRowContext(ctx, sq, id), false)
	if db.IsNoRows(err) {
		return rec{}, httpx.NotFound("event")
	}
	return r, err
}

func (p *repo) queryRecs(ctx context.Context, withDist bool, sq string, args ...any) ([]rec, error) {
	rows, err := p.db.QueryContext(ctx, sq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rec
	for rows.Next() {
		r, err := scanRec(rows, withDist)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *repo) sportActive(ctx context.Context, id int64) (bool, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sports WHERE id = ? AND active = 1`, id).Scan(&n)
	return n > 0, err
}

func (p *repo) organiserApproved(ctx context.Context, userID int64) (bool, error) {
	var st string
	err := p.db.QueryRowContext(ctx, `SELECT status FROM organiser_profiles WHERE user_id = ?`, userID).Scan(&st)
	if db.IsNoRows(err) {
		return false, nil
	}
	return st == "approved", err
}

func (p *repo) countActive(ctx context.Context, q db.Querier, userID int64) (int, error) {
	var n int
	err := p.q(q).QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE organiser_id = ?
		AND status IN ('draft','pending_payment','pending_review','published')`, userID).Scan(&n)
	return n, err
}

func modesJSON(m []string) string {
	if m == nil {
		m = []string{}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// insert writes a new draft event.
func (p *repo) insert(ctx context.Context, tx *sql.Tx, r rec, phoneEnc []byte, keyVer uint16) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO events (organiser_id, sport_id, title, description, event_type,
		participant_type, team_size_min, team_size_max, competition_level, category, eligibility_type,
		eligibility_scope, eligibility_district_id, eligibility_state_id, require_verified, gender_category,
		age_min, age_max, venue_name, address_line, city, district, state, pincode, latitude, longitude,
		starts_at, ends_at, registration_deadline, entry_fee_paise, payment_modes, payment_instructions,
		max_slots, rules, prize_info, contact_name, contact_phone_enc, contact_phone_last4, key_version,
		banner_upload_id, status)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'draft')`,
		append(recArgs(r), phoneEnc, r.PhoneLast4, keyVer, r.BannerUploadID)...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// recArgs are the editable columns in insert/update order (sport_id .. contact_name).
func recArgs(r rec) []any {
	return []any{r.OrganiserID, r.SportID, r.Title, r.Description, r.EventType, r.ParticipantType,
		r.TeamSizeMin, r.TeamSizeMax, r.CompetitionLevel, r.Category, r.ElType, r.ElScope, r.ElDistrictID,
		r.ElStateID, r.RequireVerified, r.GenderCategory, r.AgeMin, r.AgeMax, r.VenueName, r.AddressLine,
		r.City, r.District, r.State, r.Pincode, r.Latitude, r.Longitude, r.StartsAt, r.EndsAt, r.Deadline,
		r.EntryFeePaise, modesJSON(r.PaymentModes), r.PaymentInstructions, r.MaxSlots, r.Rules, r.PrizeInfo,
		r.ContactName}
}

// update writes every editable column; phoneEnc nil keeps the stored number. The row must
// still be in status `from` (optimistic guard; the caller holds FOR UPDATE anyway).
func (p *repo) update(ctx context.Context, tx *sql.Tx, r rec, phoneEnc []byte, keyVer uint16, from, to string) error {
	args := recArgs(r)[1:] // organiser_id never changes
	var enc any
	var ver any
	if phoneEnc != nil {
		enc, ver = phoneEnc, keyVer
	}
	args = append(args, enc, r.PhoneLast4, ver, r.BannerUploadID, to, r.ID, from)
	res, err := tx.ExecContext(ctx, `UPDATE events SET sport_id = ?, title = ?, description = ?, event_type = ?,
		participant_type = ?, team_size_min = ?, team_size_max = ?, competition_level = ?, category = ?,
		eligibility_type = ?, eligibility_scope = ?, eligibility_district_id = ?, eligibility_state_id = ?,
		require_verified = ?, gender_category = ?, age_min = ?, age_max = ?, venue_name = ?, address_line = ?,
		city = ?, district = ?, state = ?, pincode = ?, latitude = ?, longitude = ?, starts_at = ?, ends_at = ?,
		registration_deadline = ?, entry_fee_paise = ?, payment_modes = ?, payment_instructions = ?,
		max_slots = ?, rules = ?, prize_info = ?, contact_name = ?,
		contact_phone_enc = COALESCE(?, contact_phone_enc), contact_phone_last4 = ?,
		key_version = COALESCE(?, key_version), banner_upload_id = ?, status = ?
		WHERE id = ? AND status = ?`, args...)
	return affectedOne(res, err)
}

func affectedOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.InvalidState("The event changed meanwhile. Reload and try again.")
	}
	return nil
}

// transition moves an event from one of `from` to `to`, setting the reason column and timestamps.
func (p *repo) transition(ctx context.Context, tx *sql.Tx, id int64, from []string, to, reason string, now time.Time) error {
	set := `status = ?`
	args := []any{to}
	switch to {
	case stCancelled, stRemoved:
		set += `, cancel_reason = ?, cancelled_at = ?`
		args = append(args, reason, now)
	case stRejected:
		set += `, rejection_reason = ?`
		args = append(args, reason)
	case stPublished:
		set += `, published_at = COALESCE(published_at, ?)`
		args = append(args, now)
	case stCompleted:
		set += `, completed_at = ?`
		args = append(args, now)
	}
	fs := make([]any, len(from))
	for i, f := range from {
		fs[i] = f
	}
	args = append(append(args, id), fs...)
	res, err := tx.ExecContext(ctx, `UPDATE events SET `+set+` WHERE id = ? AND status IN (`+db.Placeholders(len(from))+`)`, args...)
	return affectedOne(res, err)
}

func (p *repo) phoneEnc(ctx context.Context, id int64) ([]byte, error) {
	var b []byte
	err := p.db.QueryRowContext(ctx, `SELECT contact_phone_enc FROM events WHERE id = ?`, id).Scan(&b)
	return b, err
}

// myRegistration reads the caller's registration (table owned by registration; read-only here).
func (p *repo) myRegistration(ctx context.Context, eventID, userID int64) (*Registration, error) {
	var g Registration
	var members []byte
	err := p.db.QueryRowContext(ctx, `SELECT id, event_id, status, payment_status, payment_note, team_name,
		team_members, created_at, qualified_via, qualified_district_id, eligibility_flag, eligibility_flagged_at
		FROM event_registrations WHERE event_id = ? AND user_id = ?`, eventID, userID).
		Scan(&g.ID, &g.EventID, &g.Status, &g.PaymentStatus, &g.PaymentNote, &g.TeamName, &members, &g.CreatedAt,
			&g.QualifiedVia, &g.qualifiedDistrictID, &g.EligibilityFlag, &g.EligibilityFlaggedAt)
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.TeamMembers = []TeamMember{}
	if len(members) > 0 {
		_ = json.Unmarshal(members, &g.TeamMembers)
	}
	return &g, nil
}

func (p *repo) hasGroupChat(ctx context.Context, eventID int64) (bool, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE type = 'event' AND event_id = ? AND status = 'active'`, eventID).Scan(&n)
	return n > 0, err
}

// latestListing returns the newest listing order per event (table owned by listingpayment; read-only).
func (p *repo) latestListing(ctx context.Context, ids []int64) (map[int64]ListingRef, error) {
	out := map[int64]ListingRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := p.db.QueryContext(ctx, `SELECT o.event_id, o.id, o.status, o.total_paise FROM listing_orders o
		WHERE o.id IN (SELECT MAX(id) FROM listing_orders WHERE event_id IN (`+db.Placeholders(len(ids))+`) GROUP BY event_id)`,
		db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eid int64
		var l ListingRef
		if err := rows.Scan(&eid, &l.OrderID, &l.Status, &l.TotalPaise); err != nil {
			return nil, err
		}
		out[eid] = l
	}
	return out, rows.Err()
}

// listingOrders returns every order of one event, newest first (admin view; read-only).
func (p *repo) listingOrders(ctx context.Context, eventID int64, eventStatus string) ([]ListingOrder, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT o.id, o.event_id, o.status, o.fee_rule_id, COALESCE(r.name,''),
		COALESCE(r.min_fee_paise,0), r.max_fee_paise, o.entry_fee_paise, o.max_slots, o.percent_bp, o.gst_bp,
		o.base_fee_paise, o.gst_paise, o.total_paise, o.provider, o.provider_order_id, o.refund_paise,
		o.expires_at, o.paid_at
		FROM listing_orders o LEFT JOIN listing_fee_rules r ON r.id = o.fee_rule_id
		WHERE o.event_id = ? ORDER BY o.id DESC LIMIT 50`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListingOrder{}
	for rows.Next() {
		var o ListingOrder
		q := &o.Quote
		if err := rows.Scan(&o.OrderID, &o.EventID, &o.Status, &q.FeeRuleID, &q.RuleName, &q.MinFeePaise,
			&q.MaxFeePaise, &q.EntryFeePaise, &q.MaxSlots, &q.PercentBp, &q.GstBp, &q.BaseFeePaise, &q.GstPaise,
			&q.TotalPaise, &o.Provider, &o.ProviderOrderID, &o.RefundPaise, &o.ExpiresAt, &o.PaidAt); err != nil {
			return nil, err
		}
		q.GrossEntryPaise = q.EntryFeePaise * q.MaxSlots
		q.IsFreeEvent = q.EntryFeePaise == 0
		o.EventStatus = eventStatus
		out = append(out, o)
	}
	return out, rows.Err()
}

// featuredCount is how many of the organiser's events were featured in [from, to).
func (p *repo) featuredCount(ctx context.Context, q db.Querier, userID int64, from, to time.Time) (int, error) {
	var n int
	err := p.q(q).QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE organiser_id = ? AND featured_at >= ? AND featured_at < ?`,
		userID, from, to).Scan(&n)
	return n, err
}

func (p *repo) setFeatured(ctx context.Context, tx *sql.Tx, id int64, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE events SET featured_at = ? WHERE id = ? AND status = 'published' AND featured_at IS NULL`, now, id)
	return affectedOne(res, err)
}

// clearFeatured takes an event off the featured list.
func (p *repo) clearFeatured(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE events SET featured_at = NULL WHERE id = ? AND featured_at IS NOT NULL`, id)
	return affectedOne(res, err)
}

// profileFacts returns the caller's verified birth year and gender (unknown = nil / "").
func (p *repo) profileFacts(ctx context.Context, userID int64) (*int, string, error) {
	var by *int
	err := p.db.QueryRowContext(ctx, `SELECT birth_year FROM profiles WHERE user_id = ?`, userID).Scan(&by)
	if err != nil && !db.IsNoRows(err) {
		return nil, "", err
	}
	var g *string
	err = p.db.QueryRowContext(ctx, `SELECT gender FROM identity_verifications WHERE user_id = ? AND status = 'verified'
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

// listQuery builds GET /events. eligibility is the FeedFilter condition ("" = none).
func listQuery(viewer int64, f ListFilter, pg httpx.PageParams, now time.Time, by *int, gender, elCond string, elArgs []any) (string, []any) {
	var sb strings.Builder
	args := []any{}
	withDist := f.Lat != nil && f.Lng != nil
	sb.WriteString(`SELECT ` + eventCols)
	if withDist {
		sb.WriteString(`, ` + haversine + ` AS dist`)
		args = append(args, *f.Lat, *f.Lat, *f.Lng)
	}
	sb.WriteString(eventFrom + ` JOIN users u ON u.id = e.organiser_id WHERE e.status = 'published' AND u.status = 'active' AND` + notBlocked)
	args = append(args, viewer, viewer)
	add := func(cond string, a ...any) {
		sb.WriteString(` AND ` + cond)
		args = append(args, a...)
	}
	if f.Q != "" {
		like := "%" + escapeLike(f.Q) + "%"
		add(`(MATCH(e.title, e.description, e.venue_name) AGAINST (? IN NATURAL LANGUAGE MODE) OR e.title LIKE ?)`, f.Q, like)
	}
	if f.SportID > 0 {
		add(`e.sport_id = ?`, f.SportID)
	}
	if f.EventType != "" {
		add(`e.event_type = ?`, f.EventType)
	}
	if f.CompetitionLevel != "" {
		add(`e.competition_level = ?`, f.CompetitionLevel)
	}
	if f.GenderCategory != "" {
		add(`e.gender_category = ?`, f.GenderCategory)
	}
	if f.Category != "" {
		add(`e.category = ?`, f.Category)
	}
	if f.State != "" {
		add(`e.state = ?`, f.State)
	}
	if f.District != "" {
		add(`e.district = ?`, f.District)
	}
	if f.City != "" {
		add(`e.city = ?`, f.City)
	}
	if f.From != nil {
		add(`e.starts_at >= ?`, *f.From)
	}
	if f.To != nil {
		add(`e.starts_at < ?`, *f.To)
	}
	if f.MaxEntryFeePaise != nil {
		add(`e.entry_fee_paise <= ?`, *f.MaxEntryFeePaise)
	}
	if !f.IncludeClosed {
		add(`e.registration_deadline > ?`, now)
	}
	if f.EligibleForMe {
		if by != nil {
			add(`(e.age_min IS NULL OR YEAR(e.starts_at) - ? >= e.age_min)`, *by)
			add(`(e.age_max IS NULL OR YEAR(e.starts_at) - ? <= e.age_max)`, *by)
		}
		switch gender {
		case "M":
			sb.WriteString(` AND e.gender_category <> 'female'`)
		case "F":
			sb.WriteString(` AND e.gender_category <> 'male'`)
		}
	}
	if elCond != "" {
		add(elCond, elArgs...)
	}
	if withDist && (f.RadiusKm > 0 || f.SortDistance) {
		sb.WriteString(` AND e.latitude IS NOT NULL AND e.longitude IS NOT NULL`)
	}
	if withDist && f.RadiusKm > 0 {
		add(haversine+` <= ?`, *f.Lat, *f.Lat, *f.Lng, f.RadiusKm)
	}
	if f.SortDistance {
		if d, err := strconv.ParseFloat(pg.Cursor.Time, 64); err == nil && pg.Cursor.Time != "" {
			add(`(`+haversine+` > ? OR (`+haversine+` = ? AND e.id > ?))`,
				*f.Lat, *f.Lat, *f.Lng, d, *f.Lat, *f.Lat, *f.Lng, d, pg.Cursor.ID)
		}
		sb.WriteString(` ORDER BY dist ASC, e.id ASC LIMIT ?`)
	} else {
		if pg.Cursor.Time != "" {
			if t, err := time.Parse(time.RFC3339Nano, pg.Cursor.Time); err == nil {
				add(`(e.starts_at > ? OR (e.starts_at = ? AND e.id > ?))`, t, t, pg.Cursor.ID)
			}
		}
		sb.WriteString(` ORDER BY e.starts_at ASC, e.id ASC LIMIT ?`)
	}
	args = append(args, pg.Limit+1)
	return sb.String(), args
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (p *repo) list(ctx context.Context, viewer int64, f ListFilter, pg httpx.PageParams, now time.Time, by *int, gender, elCond string, elArgs []any) ([]rec, error) {
	sq, args := listQuery(viewer, f, pg, now, by, gender, elCond, elArgs)
	return p.queryRecs(ctx, f.Lat != nil && f.Lng != nil, sq, args...)
}

func (p *repo) featured(ctx context.Context, viewer, sportID int64, state string, limit int, now time.Time, elCond string, elArgs []any) ([]rec, error) {
	sq := `SELECT ` + eventCols + eventFrom + ` JOIN users u ON u.id = e.organiser_id
		WHERE e.status = 'published' AND e.featured_at IS NOT NULL AND e.registration_deadline > ? AND u.status = 'active' AND` + notBlocked
	args := []any{now, viewer, viewer}
	if sportID > 0 {
		sq += ` AND e.sport_id = ?`
		args = append(args, sportID)
	}
	if state != "" {
		sq += ` AND e.state = ?`
		args = append(args, state)
	}
	if elCond != "" {
		sq += ` AND ` + elCond
		args = append(args, elArgs...)
	}
	sq += ` ORDER BY e.featured_at DESC, e.id DESC LIMIT ?`
	args = append(args, limit)
	return p.queryRecs(ctx, false, sq, args...)
}

func (p *repo) mine(ctx context.Context, userID int64, status string, pg httpx.PageParams) ([]rec, error) {
	sq := `SELECT ` + eventCols + eventFrom + ` WHERE e.organiser_id = ?`
	args := []any{userID}
	if status != "" {
		sq += ` AND e.status = ?`
		args = append(args, status)
	}
	if pg.Cursor.ID > 0 {
		sq += ` AND e.id < ?`
		args = append(args, pg.Cursor.ID)
	}
	sq += ` ORDER BY e.id DESC LIMIT ?`
	args = append(args, pg.Limit+1)
	return p.queryRecs(ctx, false, sq, args...)
}

func (p *repo) adminList(ctx context.Context, f AdminFilter, pg httpx.PageParams) ([]rec, error) {
	sq := `SELECT ` + eventCols + eventFrom + ` WHERE 1=1`
	var args []any
	if f.Status != "" {
		sq += ` AND e.status = ?`
		args = append(args, f.Status)
	}
	if f.SportID > 0 {
		sq += ` AND e.sport_id = ?`
		args = append(args, f.SportID)
	}
	if f.Q != "" {
		if id, ok := hashID(f.Q); ok {
			sq += ` AND e.id = ?`
			args = append(args, id)
		} else {
			sq += ` AND e.title LIKE ?`
			args = append(args, "%"+escapeLike(f.Q)+"%")
		}
	}
	if pg.Cursor.ID > 0 {
		sq += ` AND e.id < ?`
		args = append(args, pg.Cursor.ID)
	}
	sq += ` ORDER BY e.id DESC LIMIT ?`
	args = append(args, pg.Limit+1)
	return p.queryRecs(ctx, false, sq, args...)
}

// pastPublished returns ids of published events whose end passed (job batch).
func (p *repo) pastPublished(ctx context.Context, now time.Time, limit int) ([]rec, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, organiser_id, sport_id FROM events WHERE status = 'published' AND ends_at < ?
		ORDER BY id LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.ID, &r.OrganiserID, &r.SportID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *repo) complete(ctx context.Context, id int64, now time.Time) (bool, error) {
	res, err := p.db.ExecContext(ctx, `UPDATE events SET status = 'completed', completed_at = ? WHERE id = ? AND status = 'published'`, now, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// hashID parses "#123" or "123" (admin search by id).
func hashID(q string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(q), "#"), 10, 64)
	return n, err == nil && n > 0
}
