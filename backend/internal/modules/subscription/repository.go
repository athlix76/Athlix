package subscription

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{}

const planCols = `id, code, audience, name, description, price_paise, billing_period,
	store_product_id_android, store_product_id_ios, active, sort_order`

const subCols = `id, user_id, plan_id, store, store_transaction_id, status, auto_renew, started_at, expires_at`

type scanner interface{ Scan(dest ...any) error }

func scanPlan(s scanner) (AdminPlan, error) {
	var p AdminPlan
	var desc, android, ios sql.NullString
	if err := s.Scan(&p.ID, &p.Code, &p.Audience, &p.Name, &desc, &p.PricePaise, &p.BillingPeriod,
		&android, &ios, &p.Active, &p.SortOrder); err != nil {
		return p, err
	}
	p.Description = nullStr(desc)
	p.StoreProductIDAndroid = nullStr(android)
	p.StoreProductIDIos = nullStr(ios)
	p.Benefits = []Benefit{}
	return p, nil
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func scanSub(s scanner) (subRow, error) {
	var r subRow
	var txn sql.NullString
	err := s.Scan(&r.ID, &r.UserID, &r.PlanID, &r.Store, &txn, &r.Status, &r.AutoRenew, &r.StartedAt, &r.ExpiresAt)
	r.StoreTransactionID = nullStr(txn)
	return r, err
}

// ---- plans ----

// listPlans returns plans ordered by sort_order, id. audience "" = all; activeOnly hides inactive.
func (repo) listPlans(ctx context.Context, q db.Querier, audience string, activeOnly bool) ([]AdminPlan, error) {
	query := `SELECT ` + planCols + ` FROM premium_plans WHERE 1=1`
	var args []any
	if activeOnly {
		query += ` AND active = 1`
	}
	if audience != "" {
		query += ` AND audience = ?`
		args = append(args, audience)
	}
	query += ` ORDER BY sort_order, id`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminPlan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (repo) getPlan(ctx context.Context, q db.Querier, id int64) (AdminPlan, error) {
	return scanPlan(q.QueryRowContext(ctx, `SELECT `+planCols+` FROM premium_plans WHERE id = ?`, id))
}

// planByProduct finds the plan sold under a store product id. Inactive plans still match so
// renewals of a retired plan keep working; an active plan wins over an inactive one.
func (repo) planByProduct(ctx context.Context, q db.Querier, store, productID string) (AdminPlan, error) {
	col := "store_product_id_android"
	if store == StoreAppStore {
		col = "store_product_id_ios"
	}
	return scanPlan(q.QueryRowContext(ctx,
		`SELECT `+planCols+` FROM premium_plans WHERE `+col+` = ? ORDER BY active DESC, id DESC LIMIT 1`, productID))
}

// benefits returns benefit rows per plan id, ordered by sort_order.
func (repo) benefits(ctx context.Context, q db.Querier, planIDs []int64) (map[int64][]benefitRow, error) {
	out := map[int64][]benefitRow{}
	if len(planIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT plan_id, benefit_key, value, label, sort_order FROM premium_plan_benefits
		WHERE plan_id IN (`+db.Placeholders(len(planIDs))+`) ORDER BY plan_id, sort_order, id`, db.Int64Args(planIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b benefitRow
		var v []byte
		if err := rows.Scan(&b.PlanID, &b.Key, &v, &b.Label, &b.SortOrder); err != nil {
			return nil, err
		}
		b.Value = append([]byte(nil), v...)
		out[b.PlanID] = append(out[b.PlanID], b)
	}
	return out, rows.Err()
}

// withBenefits fills Benefits on every plan in place.
func (r repo) withBenefits(ctx context.Context, q db.Querier, plans []AdminPlan) error {
	ids := make([]int64, len(plans))
	for i, p := range plans {
		ids[i] = p.ID
	}
	m, err := r.benefits(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range plans {
		plans[i].Benefits = toBenefits(m[plans[i].ID])
	}
	return nil
}

func toBenefits(rows []benefitRow) []Benefit {
	out := make([]Benefit, 0, len(rows))
	for _, b := range rows {
		out = append(out, Benefit{Key: b.Key, Value: b.Value, Label: b.Label})
	}
	return out
}

func (repo) insertPlan(ctx context.Context, q db.Querier, p AdminPlan) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO premium_plans (code, audience, name, description, price_paise, billing_period,
		store_product_id_android, store_product_id_ios, active, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Code, p.Audience, p.Name, ptrArg(p.Description), p.PricePaise, p.BillingPeriod,
		ptrArg(p.StoreProductIDAndroid), ptrArg(p.StoreProductIDIos), p.Active, p.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (repo) updatePlan(ctx context.Context, q db.Querier, p AdminPlan) error {
	_, err := q.ExecContext(ctx, `UPDATE premium_plans SET code = ?, audience = ?, name = ?, description = ?, price_paise = ?,
		billing_period = ?, store_product_id_android = ?, store_product_id_ios = ?, active = ?, sort_order = ? WHERE id = ?`,
		p.Code, p.Audience, p.Name, ptrArg(p.Description), p.PricePaise, p.BillingPeriod,
		ptrArg(p.StoreProductIDAndroid), ptrArg(p.StoreProductIDIos), p.Active, p.SortOrder, p.ID)
	return err
}

func (repo) replaceBenefits(ctx context.Context, q db.Querier, planID int64, list []benefitRow) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM premium_plan_benefits WHERE plan_id = ?`, planID); err != nil {
		return err
	}
	for _, b := range list {
		if _, err := q.ExecContext(ctx, `INSERT INTO premium_plan_benefits (plan_id, benefit_key, value, label, sort_order) VALUES (?, ?, ?, ?, ?)`,
			planID, b.Key, string(b.Value), b.Label, b.SortOrder); err != nil {
			return err
		}
	}
	return nil
}

func ptrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// ---- subscriptions ----

// currentForUser is the live subscription that expires last (nil if none).
func (repo) currentForUser(ctx context.Context, q db.Querier, userID int64, now time.Time) (*subRow, error) {
	r, err := scanSub(q.QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions
		WHERE user_id = ? AND status IN ('active','grace') AND expires_at > ? ORDER BY expires_at DESC, id DESC LIMIT 1`, userID, now))
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (repo) getSub(ctx context.Context, q db.Querier, id int64) (subRow, error) {
	return scanSub(q.QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions WHERE id = ?`, id))
}

// subByStoreTxn locks the row for (store, store_transaction_id) inside a transaction.
func (repo) subByStoreTxn(ctx context.Context, q db.Querier, store, txnID string) (*subRow, error) {
	r, err := scanSub(q.QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions
		WHERE store = ? AND store_transaction_id = ? FOR UPDATE`, store, txnID))
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// subByTokenHash locks the newest Google Play row whose purchase token hashes to h.
func (repo) subByTokenHash(ctx context.Context, q db.Querier, h []byte) (*subRow, error) {
	r, err := scanSub(q.QueryRowContext(ctx, `SELECT `+subCols+` FROM subscriptions
		WHERE store = 'google_play' AND purchase_token_hash = ? ORDER BY id DESC LIMIT 1 FOR UPDATE`, h))
	if db.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (repo) insertSub(ctx context.Context, q db.Querier, s subRow, tokenHash []byte) (int64, error) {
	var h any
	if tokenHash != nil {
		h = tokenHash
	}
	res, err := q.ExecContext(ctx, `INSERT INTO subscriptions (user_id, plan_id, store, store_transaction_id, purchase_token_hash,
		status, auto_renew, started_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, s.PlanID, s.Store, ptrArg(s.StoreTransactionID), h, s.Status, s.AutoRenew, s.StartedAt, s.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// updateSubState writes the store-derived state of a row.
func (repo) updateSubState(ctx context.Context, q db.Querier, s subRow) error {
	_, err := q.ExecContext(ctx, `UPDATE subscriptions SET plan_id = ?, status = ?, auto_renew = ?, expires_at = ? WHERE id = ?`,
		s.PlanID, s.Status, s.AutoRenew, s.ExpiresAt, s.ID)
	return err
}

// setStatus changes only the status (used to cancel a replaced Play purchase).
func (repo) setStatus(ctx context.Context, q db.Querier, id int64, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE subscriptions SET status = ? WHERE id = ?`, status, id)
	return err
}

func (repo) listSubs(ctx context.Context, q db.Querier, f adminSubFilter, cursorID int64, limit int) ([]subRow, error) {
	var where []string
	var args []any
	if f.UserID > 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.PlanID > 0 {
		where = append(where, "plan_id = ?")
		args = append(args, f.PlanID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if cursorID > 0 {
		where = append(where, "id < ?")
		args = append(args, cursorID)
	}
	query := `SELECT ` + subCols + ` FROM subscriptions`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []subRow{}
	for rows.Next() {
		r, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// dueForExpiry returns live rows whose expires_at has passed (oldest id first).
func (repo) dueForExpiry(ctx context.Context, q db.Querier, now time.Time, limit int) ([]subRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+subCols+` FROM subscriptions
		WHERE status IN ('active','grace') AND expires_at <= ? ORDER BY id LIMIT ? FOR UPDATE`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []subRow{}
	for rows.Next() {
		r, err := scanSub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (repo) expire(ctx context.Context, q db.Querier, ids []int64, now time.Time) error {
	args := append(db.Int64Args(ids), now)
	_, err := q.ExecContext(ctx, `UPDATE subscriptions SET status = 'expired'
		WHERE id IN (`+db.Placeholders(len(ids))+`) AND status IN ('active','grace') AND expires_at <= ?`, args...)
	return err
}

// ---- webhook idempotency ----

// claimWebhook records (provider, eventID). It returns false when the event was already
// processed or ignored (a duplicate); a previously failed or unfinished event is retried.
func (repo) claimWebhook(ctx context.Context, q db.Querier, provider, eventID, eventType string) (bool, error) {
	_, err := q.ExecContext(ctx, `INSERT INTO payment_webhook_events (provider, provider_event_id, event_type, status) VALUES (?, ?, ?, 'received')`,
		provider, eventID, eventType)
	if err == nil {
		return true, nil
	}
	if !db.IsDuplicate(err) {
		return false, err
	}
	var status string
	if err := q.QueryRowContext(ctx, `SELECT status FROM payment_webhook_events WHERE provider = ? AND provider_event_id = ?`,
		provider, eventID).Scan(&status); err != nil {
		return false, err
	}
	return status == webhookReceived || status == webhookFailed, nil
}

func (repo) finishWebhook(ctx context.Context, q db.Querier, provider, eventID, status, entityID string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE payment_webhook_events SET status = ?, entity_id = ?, processed_at = ?
		WHERE provider = ? AND provider_event_id = ?`, status, db.NullString(entityID), now, provider, eventID)
	return err
}
