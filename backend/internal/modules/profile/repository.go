package profile

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"athlix/backend/internal/platform/db"
)

type repo struct{ db *sql.DB }

// ---- onboarding / profile row ----

// lockStep locks the users row and returns the onboarding step.
func (r *repo) lockStep(ctx context.Context, q db.Querier, userID int64) (string, error) {
	var step string
	err := q.QueryRowContext(ctx, `SELECT onboarding_step FROM users WHERE id = ? AND status <> 'deleted' FOR UPDATE`, userID).Scan(&step)
	return step, err
}

// activeSport reports whether sportID is an active sport.
func (r *repo) activeSport(ctx context.Context, q db.Querier, sportID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sports WHERE id = ? AND active = 1`, sportID).Scan(&n)
	return n > 0, err
}

// activePosition reports whether positionID is an active position of sportID.
func (r *repo) activePosition(ctx context.Context, q db.Querier, sportID, positionID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sport_positions WHERE id = ? AND sport_id = ? AND active = 1`, positionID, sportID).Scan(&n)
	return n > 0, err
}

type newProfile struct {
	UserID                 int64
	SportID                int64
	PositionID             *int64
	City, District, State  string
	Bio, AcademyClub       string
	PhotoUploadID          *int64
	BirthYear              *int
	Visibility, StatsVisib string
}

func (r *repo) insertProfile(ctx context.Context, q db.Querier, p newProfile) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO profiles (user_id, bio, sport_id, position_id, city, district, state, academy_club, photo_upload_id, birth_year, visibility, stats_visibility) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, db.NullString(p.Bio), p.SportID, nullPtr(p.PositionID), db.NullString(p.City), db.NullString(p.District), db.NullString(p.State),
		db.NullString(p.AcademyClub), nullPtr(p.PhotoUploadID), nullInt(p.BirthYear), p.Visibility, p.StatsVisib)
	return err
}

func (r *repo) insertSport(ctx context.Context, q db.Querier, userID, sportID int64, positionID *int64, primary bool) error {
	_, err := q.ExecContext(ctx, `INSERT INTO profile_sports (user_id, sport_id, position_id, is_primary) VALUES (?, ?, ?, ?)`, userID, sportID, nullPtr(positionID), primary)
	return err
}

func (r *repo) deleteSports(ctx context.Context, q db.Querier, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM profile_sports WHERE user_id = ?`, userID)
	return err
}

const profileCols = `bio, sport_id, position_id, city, district, state, academy_club, photo_upload_id, birth_year, visibility, stats_visibility, coaching_since_year, followers_count, following_count`

// load reads a profile row (sql.ErrNoRows when the user has none).
func (r *repo) load(ctx context.Context, q db.Querier, userID int64, forUpdate bool) (row, error) {
	query := `SELECT ` + profileCols + ` FROM profiles WHERE user_id = ?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var p row
	err := q.QueryRowContext(ctx, query, userID).Scan(&p.Bio, &p.SportID, &p.PositionID, &p.City, &p.District, &p.State, &p.AcademyClub,
		&p.PhotoUploadID, &p.BirthYear, &p.Visibility, &p.StatsVisibility, &p.CoachingSinceYear, &p.FollowersCount, &p.FollowingCount)
	return p, err
}

// update applies a SET list built by the service. Column names come only from the fixed
// updatable map below, never from the request.
func (r *repo) update(ctx context.Context, q db.Querier, userID int64, cols []string, args []any) error {
	if len(cols) == 0 {
		return nil
	}
	sets := make([]string, len(cols))
	for i, c := range cols {
		sets[i] = updatable[c] + ` = ?`
	}
	_, err := q.ExecContext(ctx, `UPDATE profiles SET `+strings.Join(sets, ", ")+` WHERE user_id = ?`, append(args, userID)...)
	return err
}

// updatable maps service field names to profiles columns (the dynamic SET allowlist).
var updatable = map[string]string{
	"bio": "bio", "sportId": "sport_id", "positionId": "position_id", "city": "city", "district": "district",
	"state": "state", "academyClub": "academy_club", "photoUploadId": "photo_upload_id",
	"visibility": "visibility", "statsVisibility": "stats_visibility", "coachingSinceYear": "coaching_since_year",
}

// sports lists a user's sports, primary first.
func (r *repo) sports(ctx context.Context, q db.Querier, userID int64) ([]ProfileSport, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.code, s.name, sp.id, sp.code, sp.name, ps.is_primary FROM profile_sports ps JOIN sports s ON s.id = ps.sport_id LEFT JOIN sport_positions sp ON sp.id = ps.position_id WHERE ps.user_id = ? ORDER BY ps.is_primary DESC, s.sort_order, s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProfileSport{}
	for rows.Next() {
		var ps ProfileSport
		var pid sql.NullInt64
		var pcode, pname sql.NullString
		if err := rows.Scan(&ps.Sport.ID, &ps.Sport.Code, &ps.Sport.Name, &pid, &pcode, &pname, &ps.IsPrimary); err != nil {
			return nil, err
		}
		if pid.Valid {
			ps.Position = &SportRef{ID: pid.Int64, Code: pcode.String, Name: pname.String}
		}
		out = append(out, ps)
	}
	return out, rows.Err()
}

// consultationOffered reads coach_consultations.offered (false when no row).
func (r *repo) consultationOffered(ctx context.Context, userID int64) (bool, error) {
	var offered bool
	err := r.db.QueryRowContext(ctx, `SELECT offered FROM coach_consultations WHERE user_id = ?`, userID).Scan(&offered)
	if db.IsNoRows(err) {
		return false, nil
	}
	return offered, err
}

// organiser reads the organiser status shown on a profile (not_started when no row).
func (r *repo) organiser(ctx context.Context, userID int64) (OrganiserInfo, error) {
	var o OrganiserInfo
	var name, typ sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT status, org_name, org_type FROM organiser_profiles WHERE user_id = ?`, userID).Scan(&o.Status, &name, &typ)
	if db.IsNoRows(err) {
		return OrganiserInfo{Status: "not_started"}, nil
	}
	if name.Valid {
		o.OrgName = &name.String
	}
	if typ.Valid {
		o.OrgType = &typ.String
	}
	return o, err
}

// acceptedConsultation reports an accepted consultation between a and b (either direction).
func (r *repo) acceptedConsultation(ctx context.Context, a, b int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM consultation_requests WHERE status = 'accepted' AND ((coach_id = ? AND player_id = ?) OR (coach_id = ? AND player_id = ?))`, a, b, b, a).Scan(&n)
	return n > 0, err
}

// ---- profile views ----

func (r *repo) recordView(ctx context.Context, viewedID, viewerID int64, day string) error {
	_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO profile_views (viewed_id, viewer_id, viewed_on) VALUES (?, ?, ?)`, viewedID, viewerID, day)
	return err
}

type viewRow struct {
	ViewerID int64
	Day      string
}

// viewers lists distinct viewers of userID since day `since`, latest view first, excluding
// blocks either way and users who are no longer active. after is the keyset cursor.
func (r *repo) viewers(ctx context.Context, userID int64, since string, afterDay string, afterID int64, limit int) ([]viewRow, error) {
	query := `SELECT v.viewer_id, MAX(v.viewed_on) AS last_on FROM profile_views v JOIN users u ON u.id = v.viewer_id
		WHERE v.viewed_id = ? AND v.viewed_on >= ? AND u.status = 'active'
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id = v.viewed_id AND b.blocked_id = v.viewer_id) OR (b.blocker_id = v.viewer_id AND b.blocked_id = v.viewed_id))
		GROUP BY v.viewer_id`
	args := []any{userID, since}
	if afterDay != "" {
		query += ` HAVING (last_on < ? OR (last_on = ? AND v.viewer_id < ?))`
		args = append(args, afterDay, afterDay, afterID)
	}
	query += ` ORDER BY last_on DESC, v.viewer_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []viewRow{}
	for rows.Next() {
		var v viewRow
		var day time.Time
		if err := rows.Scan(&v.ViewerID, &day); err != nil {
			return nil, err
		}
		v.Day = day.Format(dateLayout)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- stats ----

// templateFields returns the fields JSON of a template of sportID (active only when asked).
func (r *repo) templateFields(ctx context.Context, q db.Querier, templateID, sportID int64, activeOnly bool) ([]byte, error) {
	query := `SELECT fields FROM stat_templates WHERE id = ? AND sport_id = ?`
	if activeOnly {
		query += ` AND active = 1`
	}
	var b []byte
	err := q.QueryRowContext(ctx, query, templateID, sportID).Scan(&b)
	return b, err
}

// templatesFields returns fields JSON by template id.
func (r *repo) templatesFields(ctx context.Context, ids []int64) (map[int64][]byte, error) {
	out := map[int64][]byte{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, fields FROM stat_templates WHERE id IN (`+db.Placeholders(len(ids))+`)`, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var b []byte
		if err := rows.Scan(&id, &b); err != nil {
			return nil, err
		}
		out[id] = b
	}
	return out, rows.Err()
}

const statCols = `id, sport_id, template_id, season, stats, updated_at`

func scanStat(s interface{ Scan(...any) error }) (statRow, error) {
	var r statRow
	err := s.Scan(&r.ID, &r.SportID, &r.TemplateID, &r.Season, &r.Stats, &r.UpdatedAt)
	return r, err
}

func (r *repo) listStats(ctx context.Context, userID int64) ([]statRow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+statCols+` FROM user_stats WHERE user_id = ? ORDER BY sport_id, season DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []statRow{}
	for rows.Next() {
		s, err := scanStat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// stat reads one entry scoped to its owner.
func (r *repo) stat(ctx context.Context, q db.Querier, id, userID int64, forUpdate bool) (statRow, error) {
	query := `SELECT ` + statCols + ` FROM user_stats WHERE id = ? AND user_id = ?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanStat(q.QueryRowContext(ctx, query, id, userID))
}

func (r *repo) insertStat(ctx context.Context, userID, sportID, templateID int64, season string, stats []byte) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO user_stats (user_id, sport_id, template_id, season, stats) VALUES (?, ?, ?, ?, ?)`, userID, sportID, templateID, season, stats)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateStat(ctx context.Context, q db.Querier, id, userID int64, season string, stats []byte) error {
	_, err := q.ExecContext(ctx, `UPDATE user_stats SET season = ?, stats = ? WHERE id = ? AND user_id = ?`, season, stats, id, userID)
	return err
}

func (r *repo) deleteStat(ctx context.Context, id, userID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM user_stats WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- achievements ----

const achievementCols = `id, sport_id, title, description, level, achieved_on, certificate_upload_id, created_at`

func scanAchievement(s interface{ Scan(...any) error }) (achievementRow, error) {
	var a achievementRow
	err := s.Scan(&a.ID, &a.SportID, &a.Title, &a.Description, &a.Level, &a.AchievedOn, &a.CertID, &a.CreatedAt)
	return a, err
}

func (r *repo) listAchievements(ctx context.Context, userID int64) ([]achievementRow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+achievementCols+` FROM achievements WHERE user_id = ? ORDER BY achieved_on DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []achievementRow{}
	for rows.Next() {
		a, err := scanAchievement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *repo) achievement(ctx context.Context, q db.Querier, id, userID int64, forUpdate bool) (achievementRow, error) {
	query := `SELECT ` + achievementCols + ` FROM achievements WHERE id = ? AND user_id = ?`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanAchievement(q.QueryRowContext(ctx, query, id, userID))
}

// lockProfile locks the caller's profile row (serialises the achievement limit check).
func (r *repo) lockProfile(ctx context.Context, q db.Querier, userID int64) error {
	var id int64
	return q.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE user_id = ? FOR UPDATE`, userID).Scan(&id)
}

func (r *repo) countAchievements(ctx context.Context, q db.Querier, userID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM achievements WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

type achievementValues struct {
	SportID     *int64
	Title       string
	Description string
	Level       string
	AchievedOn  string
	CertID      *int64
}

func (r *repo) insertAchievement(ctx context.Context, q db.Querier, userID int64, v achievementValues) (int64, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO achievements (user_id, sport_id, title, description, level, achieved_on, certificate_upload_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, nullPtr(v.SportID), v.Title, db.NullString(v.Description), v.Level, db.NullString(v.AchievedOn), nullPtr(v.CertID))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *repo) updateAchievement(ctx context.Context, q db.Querier, id, userID int64, v achievementValues) error {
	_, err := q.ExecContext(ctx, `UPDATE achievements SET sport_id = ?, title = ?, description = ?, level = ?, achieved_on = ?, certificate_upload_id = ? WHERE id = ? AND user_id = ?`,
		nullPtr(v.SportID), v.Title, db.NullString(v.Description), v.Level, db.NullString(v.AchievedOn), nullPtr(v.CertID), id, userID)
	return err
}

func (r *repo) deleteAchievement(ctx context.Context, q db.Querier, id, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM achievements WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// ---- user deletion ----

// uploadsOf returns the photo and certificate upload ids of a user (purged after a wipe).
func (r *repo) uploadsOf(ctx context.Context, q db.Querier, userID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT photo_upload_id FROM profiles WHERE user_id = ? AND photo_upload_id IS NOT NULL UNION ALL SELECT certificate_upload_id FROM achievements WHERE user_id = ? AND certificate_upload_id IS NOT NULL`, userID, userID)
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

// wipe clears a deleted user's profile data (idempotent).
func (r *repo) wipe(ctx context.Context, q db.Querier, userID int64) error {
	stmts := []string{
		`UPDATE profiles SET bio = NULL, city = NULL, district = NULL, state = NULL, academy_club = NULL, photo_upload_id = NULL, birth_year = NULL, coaching_since_year = NULL, visibility = 'private', stats_visibility = 'private' WHERE user_id = ?`,
		`DELETE FROM profile_sports WHERE user_id = ?`,
		`DELETE FROM user_stats WHERE user_id = ?`,
		`DELETE FROM achievements WHERE user_id = ?`,
	}
	for _, s := range stmts {
		if _, err := q.ExecContext(ctx, s, userID); err != nil {
			return err
		}
	}
	_, err := q.ExecContext(ctx, `DELETE FROM profile_views WHERE viewed_id = ? OR viewer_id = ?`, userID, userID)
	return err
}

func nullPtr(v *int64) any {
	if v == nil || *v == 0 {
		return nil
	}
	return *v
}

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
