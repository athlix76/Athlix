package organiser

import (
	"context"
	"database/sql"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

const profileCols = `SELECT org_name, org_type, status, review_note, submitted_at, reviewed_at FROM organiser_profiles WHERE user_id = ?`

// profile loads organiser_profiles for userID; a missing row is returned as not_started.
func (r *repo) profile(ctx context.Context, q db.Querier, userID int64, forUpdate bool) (profileRow, error) {
	query := profileCols
	if forUpdate {
		query += " FOR UPDATE"
	}
	var p profileRow
	var name, typ, note sql.NullString
	var sub, rev sql.NullTime
	err := q.QueryRowContext(ctx, query, userID).Scan(&name, &typ, &p.Status, &note, &sub, &rev)
	if db.IsNoRows(err) {
		return profileRow{Status: StatusNotStarted}, nil
	}
	if err != nil {
		return profileRow{}, err
	}
	p.exists = true
	p.OrgName, p.OrgType, p.ReviewNote = strPtr(name), strPtr(typ), strPtr(note)
	p.SubmittedAt, p.ReviewedAt = timePtr(sub), timePtr(rev)
	return p, nil
}

// steps returns the active steps plus any inactive step the user already filled, in order.
func (r *repo) steps(ctx context.Context, q db.Querier, userID int64) ([]stepRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.step_code, s.title, s.description, s.input_type, s.is_required, s.active, v.id, v.value_text, v.document_upload_id, v.status, v.review_note FROM organiser_verification_steps s LEFT JOIN organiser_verification_submissions v ON v.step_id = s.id AND v.user_id = ? WHERE s.active = 1 OR v.id IS NOT NULL ORDER BY s.sort_order, s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []stepRow{}
	for rows.Next() {
		var s stepRow
		var desc, val, subStatus, note sql.NullString
		var subID, upload sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Code, &s.Title, &desc, &s.InputType, &s.Required, &s.Active, &subID, &val, &upload, &subStatus, &note); err != nil {
			return nil, err
		}
		s.Description, s.ValueText, s.SubNote = strPtr(desc), strPtr(val), strPtr(note)
		s.HasSub, s.SubStatus = subID.Valid, subStatus.String
		if upload.Valid {
			id := upload.Int64
			s.UploadID = &id
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// activeSteps maps active step id → input type.
func (r *repo) activeSteps(ctx context.Context, q db.Querier) (map[int64]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, input_type FROM organiser_verification_steps WHERE active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var typ string
		if err := rows.Scan(&id, &typ); err != nil {
			return nil, err
		}
		out[id] = typ
	}
	return out, rows.Err()
}

// documents maps step id → document upload id of the user's submissions (locked).
func (r *repo) documents(ctx context.Context, q db.Querier, userID int64) (map[int64]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT step_id, document_upload_id FROM organiser_verification_submissions WHERE user_id = ? AND document_upload_id IS NOT NULL FOR UPDATE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var step, upload int64
		if err := rows.Scan(&step, &upload); err != nil {
			return nil, err
		}
		out[step] = upload
	}
	return out, rows.Err()
}

func (r *repo) saveProfile(ctx context.Context, q db.Querier, userID int64, name, typ *string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO organiser_profiles (user_id, org_name, org_type, status) VALUES (?, ?, ?, 'draft') ON DUPLICATE KEY UPDATE org_name = VALUES(org_name), org_type = VALUES(org_type), status = 'draft'`,
		userID, nullable(name), nullable(typ))
	return err
}

func (r *repo) upsertSubmission(ctx context.Context, q db.Querier, userID, stepID int64, value *string, uploadID *int64) error {
	var up any
	if uploadID != nil {
		up = *uploadID
	}
	_, err := q.ExecContext(ctx, `INSERT INTO organiser_verification_submissions (user_id, step_id, value_text, document_upload_id, status) VALUES (?, ?, ?, ?, 'submitted') ON DUPLICATE KEY UPDATE value_text = VALUES(value_text), document_upload_id = VALUES(document_upload_id), status = 'submitted', reviewed_by = NULL, review_note = NULL, reviewed_at = NULL`,
		userID, stepID, nullable(value), up)
	return err
}

func (r *repo) deleteSubmission(ctx context.Context, q db.Querier, userID, stepID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM organiser_verification_submissions WHERE user_id = ? AND step_id = ?`, userID, stepID)
	return err
}

// missingRequired returns the codes of active required steps the user has not filled.
func (r *repo) missingRequired(ctx context.Context, q db.Querier, userID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.step_code FROM organiser_verification_steps s LEFT JOIN organiser_verification_submissions v ON v.step_id = s.id AND v.user_id = ? WHERE s.active = 1 AND s.is_required = 1 AND v.id IS NULL ORDER BY s.sort_order, s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (r *repo) markSubmitted(ctx context.Context, q db.Querier, userID int64, at time.Time) error {
	if _, err := q.ExecContext(ctx, `UPDATE organiser_profiles SET status = 'submitted', submitted_at = ?, reviewed_by = NULL, review_note = NULL, reviewed_at = NULL WHERE user_id = ?`, at, userID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE organiser_verification_submissions SET status = 'submitted', reviewed_by = NULL, review_note = NULL, reviewed_at = NULL WHERE user_id = ?`, userID)
	return err
}

// review records an admin decision on the profile and every submission of the user.
func (r *repo) review(ctx context.Context, q db.Querier, userID, adminID int64, status string, note *string, at time.Time) error {
	if _, err := q.ExecContext(ctx, `UPDATE organiser_profiles SET status = ?, reviewed_by = ?, review_note = ?, reviewed_at = ? WHERE user_id = ? AND status = 'submitted'`,
		status, adminID, nullable(note), at, userID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE organiser_verification_submissions SET status = ?, reviewed_by = ?, reviewed_at = ? WHERE user_id = ?`, status, adminID, at, userID)
	return err
}

// adminList pages organiser_profiles by status, newest user id first.
func (r *repo) adminList(ctx context.Context, status string, beforeID int64, limit int) ([]AdminRow, error) {
	query := `SELECT p.user_id, p.org_name, p.org_type, p.status, p.submitted_at FROM organiser_profiles p JOIN users u ON u.id = p.user_id WHERE p.status = ? AND u.status <> 'deleted'`
	args := []any{status}
	if beforeID > 0 {
		query += ` AND p.user_id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY p.user_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminRow{}
	for rows.Next() {
		var a AdminRow
		var name, typ sql.NullString
		var sub sql.NullTime
		if err := rows.Scan(&a.User.ID, &name, &typ, &a.Status, &sub); err != nil {
			return nil, err
		}
		a.OrgName, a.OrgType, a.SubmittedAt = strPtr(name), strPtr(typ), timePtr(sub)
		out = append(out, a)
	}
	return out, rows.Err()
}

// allDocuments lists every document upload of the user's submissions (for deletion).
func (r *repo) allDocuments(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT document_upload_id FROM organiser_verification_submissions WHERE user_id = ? AND document_upload_id IS NOT NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// wipe deletes the submissions and clears the organisation name of a deleted user.
func (r *repo) wipe(ctx context.Context, q db.Querier, userID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM organiser_verification_submissions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE organiser_profiles SET org_name = NULL, review_note = NULL WHERE user_id = ?`, userID)
	return err
}

func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func timePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}
