package listingpayment

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

const evCols = `e.id, e.organiser_id, e.sport_id, e.event_type, e.entry_fee_paise, e.max_slots, e.status,
	e.starts_at, e.ends_at, e.registration_deadline, e.participant_type, e.team_size_min, e.team_size_max,
	e.registrations_count`

const orderCols = `o.id, o.event_id, o.organiser_id, o.fee_rule_id, o.entry_fee_paise, o.max_slots, o.percent_bp,
	o.gst_bp, o.base_fee_paise, o.gst_paise, o.total_paise, o.provider, o.provider_order_id, o.provider_payment_id,
	o.status, o.refund_paise, o.provider_refund_id, o.paid_at, o.expires_at, o.created_at,
	COALESCE(r.name, ''), COALESCE(r.min_fee_paise, 0), r.max_fee_paise, e.status`

const orderFrom = ` FROM listing_orders o LEFT JOIN listing_fee_rules r ON r.id = o.fee_rule_id
	JOIN events e ON e.id = o.event_id`

const ruleCols = `id, name, sport_id, event_type, percent_bp, min_fee_paise, max_fee_paise, free_event_fee_paise,
	gst_bp, priority, active, effective_from, effective_to`

type scanner interface{ Scan(dest ...any) error }

func scanOrder(sc scanner) (orderRec, error) {
	var o orderRec
	err := sc.Scan(&o.ID, &o.EventID, &o.OrganiserID, &o.FeeRuleID, &o.EntryFeePaise, &o.MaxSlots, &o.PercentBp,
		&o.GstBp, &o.BaseFeePaise, &o.GstPaise, &o.TotalPaise, &o.Provider, &o.ProviderOrderID, &o.ProviderPaymentID,
		&o.Status, &o.RefundPaise, &o.ProviderRefundID, &o.PaidAt, &o.ExpiresAt, &o.CreatedAt,
		&o.RuleName, &o.MinFeePaise, &o.MaxFeePaise, &o.EventStatus)
	return o, err
}

func scanRule(sc scanner) (FeeRule, error) {
	var r FeeRule
	err := sc.Scan(&r.ID, &r.Name, &r.SportID, &r.EventType, &r.PercentBp, &r.MinFeePaise, &r.MaxFeePaise,
		&r.FreeEventFeePaise, &r.GstBp, &r.Priority, &r.Active, &r.EffectiveFrom, &r.EffectiveTo)
	return r, err
}

type repo struct{ db *sql.DB }

func (p *repo) q(q db.Querier) db.Querier {
	if q == nil {
		return p.db
	}
	return q
}

// ---------- events (read; status written only on payment / expiry) ----------

// event loads an event. organiserID > 0 scopes it to its owner (404 for anyone else).
func (p *repo) event(ctx context.Context, q db.Querier, id, organiserID int64, forUpdate bool) (evRow, error) {
	sq := `SELECT ` + evCols + ` FROM events e WHERE e.id = ?`
	args := []any{id}
	if organiserID > 0 {
		sq += ` AND e.organiser_id = ?`
		args = append(args, organiserID)
	}
	if forUpdate {
		sq += ` FOR UPDATE`
	}
	var e evRow
	err := p.q(q).QueryRowContext(ctx, sq, args...).Scan(&e.ID, &e.OrganiserID, &e.SportID, &e.EventType,
		&e.EntryFeePaise, &e.MaxSlots, &e.Status, &e.StartsAt, &e.EndsAt, &e.Deadline, &e.ParticipantType,
		&e.TeamSizeMin, &e.TeamSizeMax, &e.RegistrationsCount)
	if db.IsNoRows(err) {
		return evRow{}, httpx.NotFound("event")
	}
	return e, err
}

// setEventStatus moves the event from one status to another (false when it was not in from).
func (p *repo) setEventStatus(ctx context.Context, tx *sql.Tx, id int64, from []string, to string, now time.Time) (bool, error) {
	sq := `UPDATE events SET status = ?`
	args := []any{to}
	if to == "published" {
		sq += `, published_at = COALESCE(published_at, ?)`
		args = append(args, now)
	}
	sq += ` WHERE id = ? AND status IN (` + db.Placeholders(len(from)) + `)`
	args = append(args, id)
	for _, f := range from {
		args = append(args, f)
	}
	res, err := tx.ExecContext(ctx, sq, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// revertEvent returns a pending_payment event to draft when it has no live open order.
func (p *repo) revertEvent(ctx context.Context, tx *sql.Tx, eventID int64, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE events SET status = 'draft' WHERE id = ? AND status = 'pending_payment'
		AND NOT EXISTS (SELECT 1 FROM listing_orders o WHERE o.event_id = ? AND o.status IN ('created','failed') AND o.expires_at > ?)`,
		eventID, eventID, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// ---------- fee rules ----------

// candidateRules returns active rules in their window that match sport/type (or are wildcards).
func (p *repo) candidateRules(ctx context.Context, sportID int64, eventType string, now time.Time) ([]FeeRule, error) {
	return p.rules(ctx, nil, `SELECT `+ruleCols+` FROM listing_fee_rules
		WHERE active = 1 AND effective_from <= ? AND (effective_to IS NULL OR effective_to > ?)
		AND (sport_id IS NULL OR sport_id = ?) AND (event_type IS NULL OR event_type = ?)`,
		now, now, sportID, eventType)
}

func (p *repo) rules(ctx context.Context, q db.Querier, sq string, args ...any) ([]FeeRule, error) {
	rows, err := p.q(q).QueryContext(ctx, sq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeeRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *repo) allRules(ctx context.Context) ([]FeeRule, error) {
	return p.rules(ctx, nil, `SELECT `+ruleCols+` FROM listing_fee_rules ORDER BY active DESC, priority DESC, id DESC`)
}

func (p *repo) rule(ctx context.Context, tx *sql.Tx, id int64) (FeeRule, error) {
	r, err := scanRule(tx.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM listing_fee_rules WHERE id = ? FOR UPDATE`, id))
	if db.IsNoRows(err) {
		return FeeRule{}, httpx.NotFound("fee rule")
	}
	return r, err
}

// otherActiveDefaults counts active default rules other than id (locks them).
func (p *repo) otherActiveDefaults(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT id FROM listing_fee_rules
		WHERE active = 1 AND sport_id IS NULL AND event_type IS NULL AND id <> ? FOR UPDATE) t`, id).Scan(&n)
	return n, err
}

func ruleArgs(r FeeRule) []any {
	return []any{r.Name, r.SportID, r.EventType, r.PercentBp, r.MinFeePaise, r.MaxFeePaise, r.FreeEventFeePaise,
		r.GstBp, r.Priority, r.Active, r.EffectiveFrom, r.EffectiveTo}
}

func (p *repo) insertRule(ctx context.Context, tx *sql.Tx, r FeeRule, adminID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO listing_fee_rules (name, sport_id, event_type, percent_bp, min_fee_paise,
		max_fee_paise, free_event_fee_paise, gst_bp, priority, active, effective_from, effective_to, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, append(ruleArgs(r), adminID)...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (p *repo) updateRule(ctx context.Context, tx *sql.Tx, r FeeRule) error {
	_, err := tx.ExecContext(ctx, `UPDATE listing_fee_rules SET name = ?, sport_id = ?, event_type = ?, percent_bp = ?,
		min_fee_paise = ?, max_fee_paise = ?, free_event_fee_paise = ?, gst_bp = ?, priority = ?, active = ?,
		effective_from = ?, effective_to = ? WHERE id = ?`, append(ruleArgs(r), r.ID)...)
	return err
}

// policyDocID is the active listing_refund policy document (nil when none).
func (p *repo) policyDocID(ctx context.Context) (*int64, error) {
	var id int64
	err := p.db.QueryRowContext(ctx, `SELECT id FROM policy_documents WHERE doc_type = 'listing_refund' AND active = 1
		ORDER BY version DESC LIMIT 1`).Scan(&id)
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ---------- orders ----------

func (p *repo) orders(ctx context.Context, q db.Querier, sq string, args ...any) ([]orderRec, error) {
	rows, err := p.q(q).QueryContext(ctx, sq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []orderRec
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (p *repo) oneOrder(ctx context.Context, q db.Querier, sq string, args ...any) (orderRec, error) {
	o, err := scanOrder(p.q(q).QueryRowContext(ctx, sq, args...))
	if db.IsNoRows(err) {
		return orderRec{}, httpx.NotFound("order")
	}
	return o, err
}

// ownedOrder loads an order scoped to its organiser (and event when eventID > 0); 404 otherwise.
func (p *repo) ownedOrder(ctx context.Context, q db.Querier, id, eventID, organiserID int64, forUpdate bool) (orderRec, error) {
	sq := `SELECT ` + orderCols + orderFrom + ` WHERE o.id = ? AND o.organiser_id = ?`
	args := []any{id, organiserID}
	if eventID > 0 {
		sq += ` AND o.event_id = ?`
		args = append(args, eventID)
	}
	if forUpdate {
		sq += ` FOR UPDATE OF o`
	}
	return p.oneOrder(ctx, q, sq, args...)
}

// latestOwnedOrder loads the newest order for an event the organiser owns; 404 otherwise.
func (p *repo) latestOwnedOrder(ctx context.Context, eventID, organiserID int64) (orderRec, error) {
	sq := `SELECT ` + orderCols + orderFrom + ` WHERE o.event_id = ? AND o.organiser_id = ? ORDER BY o.id DESC LIMIT 1`
	return p.oneOrder(ctx, nil, sq, eventID, organiserID)
}

// orderByID loads any order (admin / webhook paths). Locks it when tx is set.
func (p *repo) orderByID(ctx context.Context, q db.Querier, id int64, forUpdate bool) (orderRec, error) {
	sq := `SELECT ` + orderCols + orderFrom + ` WHERE o.id = ?`
	if forUpdate {
		sq += ` FOR UPDATE OF o`
	}
	return p.oneOrder(ctx, q, sq, id)
}

// orderIDByProvider finds our order for a Razorpay order id (0 when it is not ours).
func (p *repo) orderIDByProvider(ctx context.Context, q db.Querier, providerOrderID string) (int64, int64, error) {
	var id, eventID int64
	err := p.q(q).QueryRowContext(ctx, `SELECT id, event_id FROM listing_orders WHERE provider = 'razorpay' AND provider_order_id = ?`,
		providerOrderID).Scan(&id, &eventID)
	if db.IsNoRows(err) {
		return 0, 0, nil
	}
	return id, eventID, err
}

// eventOrders returns the event's orders in the given statuses, locked.
func (p *repo) eventOrders(ctx context.Context, tx *sql.Tx, eventID int64, statuses ...string) ([]orderRec, error) {
	args := []any{eventID}
	for _, s := range statuses {
		args = append(args, s)
	}
	return p.orders(ctx, tx, `SELECT `+orderCols+orderFrom+` WHERE o.event_id = ? AND o.status IN (`+
		db.Placeholders(len(statuses))+`) ORDER BY o.id ASC FOR UPDATE OF o`, args...)
}

func (p *repo) insertOrder(ctx context.Context, tx *sql.Tx, o orderRec) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO listing_orders (event_id, organiser_id, fee_rule_id, entry_fee_paise,
		max_slots, percent_bp, gst_bp, base_fee_paise, gst_paise, total_paise, provider, status, paid_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.EventID, o.OrganiserID, o.FeeRuleID, o.EntryFeePaise, o.MaxSlots, o.PercentBp, o.GstBp, o.BaseFeePaise,
		o.GstPaise, o.TotalPaise, o.Provider, o.Status, o.PaidAt, o.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (p *repo) setProviderOrder(ctx context.Context, tx *sql.Tx, id int64, providerOrderID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE listing_orders SET provider_order_id = ? WHERE id = ?`, providerOrderID, id)
	return err
}

// expire marks open orders expired.
func (p *repo) expire(ctx context.Context, tx *sql.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE listing_orders SET status = 'expired' WHERE id IN (`+db.Placeholders(len(ids))+
		`) AND status IN ('created','failed')`, db.Int64Args(ids)...)
	return err
}

// markPaid records the payment on an open order.
func (p *repo) markPaid(ctx context.Context, tx *sql.Tx, id int64, paymentID *string, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE listing_orders SET status = 'paid', provider_payment_id = ?, paid_at = ?
		WHERE id = ? AND status IN ('created','failed','expired')`, paymentID, now, id)
	return oneRow(res, err)
}

func (p *repo) markFailed(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE listing_orders SET status = 'failed' WHERE id = ? AND status = 'created'`, id)
	return err
}

// addRefund records a refund; prevRefund guards against a concurrent refund of the same order.
func (p *repo) addRefund(ctx context.Context, tx *sql.Tx, o orderRec, amount int64, status string, refundID *string, reason string, refundedAt *time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE listing_orders SET refund_paise = refund_paise + ?, status = ?,
		provider_refund_id = COALESCE(?, provider_refund_id), refund_reason = ?, refunded_at = COALESCE(?, refunded_at)
		WHERE id = ? AND status IN ('paid','partially_refunded') AND refund_paise = ?`,
		amount, status, refundID, reason, refundedAt, o.ID, o.RefundPaise)
	return oneRow(res, err)
}

// refundProcessed stamps refunded_at when Razorpay reports the refund done (idempotent).
func (p *repo) refundProcessed(ctx context.Context, tx *sql.Tx, refundID string, now time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE listing_orders SET refunded_at = COALESCE(refunded_at, ?)
		WHERE provider = 'razorpay' AND provider_refund_id = ?`, now, refundID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// dueOrders returns open orders past expires_at, oldest first, locked.
func (p *repo) dueOrders(ctx context.Context, tx *sql.Tx, now time.Time, limit int) ([]int64, []int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, event_id FROM listing_orders WHERE status IN ('created','failed')
		AND expires_at <= ? ORDER BY id ASC LIMIT ? FOR UPDATE`, now, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids, evs []int64
	for rows.Next() {
		var id, ev int64
		if err := rows.Scan(&id, &ev); err != nil {
			return nil, nil, err
		}
		ids, evs = append(ids, id), append(evs, ev)
	}
	return ids, evs, rows.Err()
}

// adminOrders is GET /admin/listing-orders (newest first, limit+1 rows).
func (p *repo) adminOrders(ctx context.Context, f OrderFilter, pg httpx.PageParams) ([]orderRec, error) {
	sq := `SELECT ` + orderCols + orderFrom + ` WHERE 1=1`
	var args []any
	if f.Status != "" {
		sq += ` AND o.status = ?`
		args = append(args, f.Status)
	}
	if f.EventID > 0 {
		sq += ` AND o.event_id = ?`
		args = append(args, f.EventID)
	}
	if f.From != nil {
		sq += ` AND o.created_at >= ?`
		args = append(args, *f.From)
	}
	if f.To != nil {
		sq += ` AND o.created_at < ?`
		args = append(args, *f.To)
	}
	if pg.Cursor.ID > 0 {
		sq += ` AND o.id < ?`
		args = append(args, pg.Cursor.ID)
	}
	sq += ` ORDER BY o.id DESC LIMIT ?`
	return p.orders(ctx, nil, sq, append(args, pg.Limit+1)...)
}

// ---------- webhooks ----------

// recordWebhook inserts the dedupe row (ids and type only; never the payload). It returns
// false when the event was already recorded.
func (p *repo) recordWebhook(ctx context.Context, tx *sql.Tx, eventID, typ string, entityID *string, status string, now time.Time) (bool, error) {
	_, err := tx.ExecContext(ctx, `INSERT INTO payment_webhook_events (provider, provider_event_id, event_type, entity_id,
		status, received_at, processed_at) VALUES ('razorpay', ?, ?, ?, ?, ?, ?)`, eventID, typ, entityID, status, now, now)
	if db.IsDuplicate(err) {
		return false, nil
	}
	return err == nil, err
}

func oneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return httpx.Conflict("The order changed; try again.")
	}
	return nil
}
