package appconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"athlix/backend/internal/modules/sport"
	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

// ---- app_config ----

const configCols = `config_key, value, value_type, category, description, is_public, version, updated_at`

func scanConfig(sc interface{ Scan(...any) error }) (ConfigEntry, error) {
	var e ConfigEntry
	var raw []byte
	if err := sc.Scan(&e.Key, &raw, &e.ValueType, &e.Category, &e.Description, &e.IsPublic, &e.Version, &e.UpdatedAt); err != nil {
		return e, err
	}
	e.Value = json.RawMessage(raw)
	e.UpdatedAt = e.UpdatedAt.UTC()
	return e, nil
}

func (r *repo) configList(ctx context.Context, category string) ([]ConfigEntry, error) {
	q := `SELECT ` + configCols + ` FROM app_config`
	var args []any
	if category != "" {
		q += ` WHERE category = ?`
		args = append(args, category)
	}
	q += ` ORDER BY category, config_key`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigEntry{}
	for rows.Next() {
		e, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *repo) configForUpdate(ctx context.Context, tx *sql.Tx, key string) (ConfigEntry, error) {
	return scanConfig(tx.QueryRowContext(ctx, `SELECT `+configCols+` FROM app_config WHERE config_key = ? FOR UPDATE`, key))
}

func (r *repo) configExists(ctx context.Context, key string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_config WHERE config_key = ?`, key).Scan(&n)
	return n > 0, err
}

func (r *repo) updateConfig(ctx context.Context, tx *sql.Tx, key string, value json.RawMessage, version int, adminID int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE app_config SET value = ?, version = ?, updated_by = ?, updated_at = ? WHERE config_key = ?`,
		string(value), version, adminID, now, key)
	return err
}

func (r *repo) configHistory(ctx context.Context, key string, limit int) ([]ConfigChange, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT c.old_value, c.new_value, c.changed_by, COALESCE(a.name, ''), c.changed_at FROM config_audit c LEFT JOIN admin_users a ON a.id = c.changed_by WHERE c.table_name = 'app_config' AND c.record_id = ? ORDER BY c.id DESC LIMIT ?`,
		key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigChange{}
	for rows.Next() {
		var ch ConfigChange
		var oldV, newV []byte
		if err := rows.Scan(&oldV, &newV, &ch.ChangedBy.ID, &ch.ChangedBy.Name, &ch.ChangedAt); err != nil {
			return nil, err
		}
		ch.OldValue, ch.NewValue = rawOrNull(oldV), rawOrNull(newV)
		ch.ChangedAt = ch.ChangedAt.UTC()
		out = append(out, ch)
	}
	return out, rows.Err()
}

func rawOrNull(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

// ---- policy_documents ----

const policyCols = `id, doc_type, version, title, content_md, requires_acceptance, active, effective_from, created_at`

func scanPolicy(sc interface{ Scan(...any) error }) (AdminPolicy, error) {
	var p AdminPolicy
	if err := sc.Scan(&p.ID, &p.DocType, &p.Version, &p.Title, &p.ContentMd, &p.RequiresAcceptance, &p.Active, &p.EffectiveFrom, &p.CreatedAt); err != nil {
		return p, err
	}
	p.EffectiveFrom, p.CreatedAt = p.EffectiveFrom.UTC(), p.CreatedAt.UTC()
	return p, nil
}

func (r *repo) activePolicies(ctx context.Context) ([]PolicySummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, doc_type, version, title, requires_acceptance FROM policy_documents WHERE active = 1 ORDER BY doc_type, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PolicySummary{}
	for rows.Next() {
		var p PolicySummary
		if err := rows.Scan(&p.ID, &p.DocType, &p.Version, &p.Title, &p.RequiresAcceptance); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *repo) activePolicy(ctx context.Context, docType string) (AdminPolicy, error) {
	return scanPolicy(r.db.QueryRowContext(ctx, `SELECT `+policyCols+` FROM policy_documents WHERE doc_type = ? AND active = 1 ORDER BY version DESC LIMIT 1`, docType))
}

func (r *repo) policies(ctx context.Context, docType string) ([]AdminPolicy, error) {
	q := `SELECT ` + policyCols + ` FROM policy_documents`
	var args []any
	if docType != "" {
		q += ` WHERE doc_type = ?`
		args = append(args, docType)
	}
	q += ` ORDER BY doc_type, version DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminPolicy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *repo) policyForUpdate(ctx context.Context, tx *sql.Tx, id int64) (AdminPolicy, error) {
	return scanPolicy(tx.QueryRowContext(ctx, `SELECT `+policyCols+` FROM policy_documents WHERE id = ? FOR UPDATE`, id))
}

// maxPolicyVersion locks the doc type's rows so two creates cannot pick the same version.
func (r *repo) maxPolicyVersion(ctx context.Context, tx *sql.Tx, docType string) (int, error) {
	var v int
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM policy_documents WHERE doc_type = ? FOR UPDATE`, docType).Scan(&v)
	return v, err
}

func (r *repo) insertPolicy(ctx context.Context, tx *sql.Tx, in createPolicyBody, version int, adminID int64, now time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO policy_documents (doc_type, version, title, content_md, requires_acceptance, active, effective_from, created_by, created_at) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		in.DocType, version, in.Title, in.ContentMd, in.RequiresAcceptance, now, adminID, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) activePolicyIDs(ctx context.Context, tx *sql.Tx, docType string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM policy_documents WHERE doc_type = ? AND active = 1 FOR UPDATE`, docType)
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

func (r *repo) activatePolicy(ctx context.Context, tx *sql.Tx, id int64, docType string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE policy_documents SET active = 0 WHERE doc_type = ? AND active = 1`, docType); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE policy_documents SET active = 1, effective_from = ? WHERE id = ?`, now, id)
	return err
}

// ---- verification_requirements ----

const reqCols = `id, role, requirement, is_required, gates`

func scanReq(sc interface{ Scan(...any) error }) (Requirement, error) {
	var q Requirement
	err := sc.Scan(&q.ID, &q.Role, &q.Requirement, &q.IsRequired, &q.Gates)
	return q, err
}

func (r *repo) requirements(ctx context.Context) ([]Requirement, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+reqCols+` FROM verification_requirements ORDER BY role, gates, requirement`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Requirement{}
	for rows.Next() {
		q, err := scanReq(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r *repo) requirementForUpdate(ctx context.Context, tx *sql.Tx, id int64) (Requirement, error) {
	return scanReq(tx.QueryRowContext(ctx, `SELECT `+reqCols+` FROM verification_requirements WHERE id = ? FOR UPDATE`, id))
}

func (r *repo) insertRequirement(ctx context.Context, tx *sql.Tx, q Requirement) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO verification_requirements (role, requirement, is_required, gates) VALUES (?, ?, ?, ?)`,
		q.Role, q.Requirement, q.IsRequired, q.Gates)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) setRequirement(ctx context.Context, tx *sql.Tx, id int64, required bool) error {
	_, err := tx.ExecContext(ctx, `UPDATE verification_requirements SET is_required = ? WHERE id = ?`, required, id)
	return err
}

// ---- organiser_verification_steps ----

const stepCols = `id, step_code, title, description, input_type, is_required, sort_order, active`

func scanStep(sc interface{ Scan(...any) error }) (OrganiserStep, error) {
	var s OrganiserStep
	var desc sql.NullString
	if err := sc.Scan(&s.ID, &s.StepCode, &s.Title, &desc, &s.InputType, &s.IsRequired, &s.SortOrder, &s.Active); err != nil {
		return s, err
	}
	if desc.Valid {
		s.Description = &desc.String
	}
	return s, nil
}

func (r *repo) steps(ctx context.Context) ([]OrganiserStep, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+stepCols+` FROM organiser_verification_steps ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrganiserStep{}
	for rows.Next() {
		s, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *repo) stepForUpdate(ctx context.Context, tx *sql.Tx, id int64) (OrganiserStep, error) {
	return scanStep(tx.QueryRowContext(ctx, `SELECT `+stepCols+` FROM organiser_verification_steps WHERE id = ? FOR UPDATE`, id))
}

func (r *repo) insertStep(ctx context.Context, tx *sql.Tx, s OrganiserStep) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO organiser_verification_steps (step_code, title, description, input_type, is_required, sort_order, active) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.StepCode, s.Title, s.Description, s.InputType, s.IsRequired, s.SortOrder, s.Active)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateStep(ctx context.Context, tx *sql.Tx, s OrganiserStep) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE organiser_verification_steps SET title = ?, description = ?, input_type = ?, is_required = ?, sort_order = ?, active = ? WHERE id = ?`,
		s.Title, s.Description, s.InputType, s.IsRequired, s.SortOrder, s.Active, s.ID)
	return err
}

// ---- sports, sport_positions, stat_templates ----

type sportRow struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	IsTeamSport bool   `json:"isTeamSport"`
	SortOrder   int    `json:"sortOrder"`
	Active      bool   `json:"active"`
}

type positionRow struct {
	ID        int64  `json:"id"`
	SportID   int64  `json:"sportId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	AppliesTo string `json:"appliesTo"`
	SortOrder int    `json:"sortOrder"`
	Active    bool   `json:"active"`
}

type templateRow struct {
	ID         int64             `json:"id"`
	SportID    int64             `json:"sportId"`
	PositionID *int64            `json:"positionId"`
	Name       string            `json:"name"`
	Fields     []sport.StatField `json:"fields"`
	Version    int               `json:"version"`
	Active     bool              `json:"active"`
}

func (r *repo) sportForUpdate(ctx context.Context, tx *sql.Tx, id int64) (sportRow, error) {
	var s sportRow
	err := tx.QueryRowContext(ctx, `SELECT id, code, name, is_team_sport, sort_order, active FROM sports WHERE id = ? FOR UPDATE`, id).
		Scan(&s.ID, &s.Code, &s.Name, &s.IsTeamSport, &s.SortOrder, &s.Active)
	return s, err
}

func (r *repo) insertSport(ctx context.Context, tx *sql.Tx, s sportRow) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO sports (code, name, is_team_sport, sort_order, active) VALUES (?, ?, ?, ?, 1)`,
		s.Code, s.Name, s.IsTeamSport, s.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateSport(ctx context.Context, tx *sql.Tx, s sportRow) error {
	_, err := tx.ExecContext(ctx, `UPDATE sports SET name = ?, is_team_sport = ?, sort_order = ?, active = ? WHERE id = ?`,
		s.Name, s.IsTeamSport, s.SortOrder, s.Active, s.ID)
	return err
}

func (r *repo) positionForUpdate(ctx context.Context, tx *sql.Tx, id int64) (positionRow, error) {
	var p positionRow
	err := tx.QueryRowContext(ctx, `SELECT id, sport_id, code, name, applies_to, sort_order, active FROM sport_positions WHERE id = ? FOR UPDATE`, id).
		Scan(&p.ID, &p.SportID, &p.Code, &p.Name, &p.AppliesTo, &p.SortOrder, &p.Active)
	return p, err
}

func (r *repo) positionSport(ctx context.Context, q db.Querier, id int64) (int64, error) {
	var sportID int64
	err := q.QueryRowContext(ctx, `SELECT sport_id FROM sport_positions WHERE id = ?`, id).Scan(&sportID)
	return sportID, err
}

func (r *repo) insertPosition(ctx context.Context, tx *sql.Tx, p positionRow) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO sport_positions (sport_id, code, name, applies_to, sort_order, active) VALUES (?, ?, ?, ?, ?, 1)`,
		p.SportID, p.Code, p.Name, p.AppliesTo, p.SortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updatePosition(ctx context.Context, tx *sql.Tx, p positionRow) error {
	_, err := tx.ExecContext(ctx, `UPDATE sport_positions SET name = ?, applies_to = ?, sort_order = ?, active = ? WHERE id = ?`,
		p.Name, p.AppliesTo, p.SortOrder, p.Active, p.ID)
	return err
}

func (r *repo) templateForUpdate(ctx context.Context, tx *sql.Tx, id int64) (templateRow, error) {
	var t templateRow
	var fields []byte
	err := tx.QueryRowContext(ctx, `SELECT id, sport_id, position_id, name, fields, version, active FROM stat_templates WHERE id = ? FOR UPDATE`, id).
		Scan(&t.ID, &t.SportID, &t.PositionID, &t.Name, &fields, &t.Version, &t.Active)
	t.Fields = sport.ParseFields(fields)
	return t, err
}

func (r *repo) insertTemplate(ctx context.Context, tx *sql.Tx, t templateRow) (int64, error) {
	b, err := json.Marshal(t.Fields)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO stat_templates (sport_id, position_id, name, fields, version, active) VALUES (?, ?, ?, ?, 1, 1)`,
		t.SportID, t.PositionID, t.Name, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateTemplate(ctx context.Context, tx *sql.Tx, t templateRow) error {
	b, err := json.Marshal(t.Fields)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE stat_templates SET name = ?, fields = ?, version = ?, active = ? WHERE id = ?`,
		t.Name, string(b), t.Version, t.Active, t.ID)
	return err
}
