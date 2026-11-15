package search

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"athlix/backend/internal/platform/httpx"
)

// repo reads other modules' tables. It never writes.
type repo struct{ db *sql.DB }

// peopleQuery holds the inputs of one people search.
type peopleQuery struct {
	Viewer        int64
	Filter        PeopleFilter
	MinorsAllowed bool
	Year          int
	Cursor        httpx.Cursor
	Limit         int
}

// mutualSQL is true when the viewer and u follow each other. Args: viewer, viewer.
const mutualSQL = `(EXISTS (SELECT 1 FROM follows f1 WHERE f1.follower_id = ? AND f1.followee_id = u.id)
	AND EXISTS (SELECT 1 FROM follows f2 WHERE f2.follower_id = u.id AND f2.followee_id = ?))`

// minorGuard limits a filter on a minor-hidden field (city, district, bio) to adults and
// mutual follows, so the filter cannot reveal what the result card hides.
const minorGuard = ` AND (u.is_minor = 0 OR ` + mutualSQL + `)`

const peopleBase = `SELECT t.id, t.bio, t.followers_count, t.is_minor, t.boosted FROM (
	SELECT u.id, p.bio, COALESCE(p.followers_count, 0) AS followers_count, u.is_minor,
	  EXISTS (SELECT 1 FROM subscriptions s JOIN premium_plan_benefits b ON b.plan_id = s.plan_id
	          WHERE s.user_id = u.id AND s.status IN ('active','grace') AND s.expires_at > CURRENT_TIMESTAMP(3)
	            AND b.benefit_key = 'profile_boost' AND JSON_UNQUOTE(b.value) NOT IN ('false','0','null')) AS boosted
	FROM users u
	LEFT JOIN profiles p ON p.user_id = u.id
	LEFT JOIN user_settings us ON us.user_id = u.id
	WHERE u.status = 'active' AND u.onboarding_step = 'done' AND u.id <> ?
	  AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(us.privacy, '$.showInSearch')), 'true') <> 'false'
	  AND NOT EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = ? AND bl.blocked_id = u.id) OR (bl.blocker_id = u.id AND bl.blocked_id = ?))`

// people returns matching users, boosted first, then newest id. Cursor: Time = boosted ("1"/"0"), ID = last id.
func (r *repo) people(ctx context.Context, pq peopleQuery) ([]personRow, error) {
	v := pq.Viewer
	f := pq.Filter
	s := peopleBase
	args := []any{v, v, v}
	if !pq.MinorsAllowed {
		s += ` AND u.is_minor = 0`
	}
	if f.Q != "" {
		like := likePattern(f.Q)
		s += ` AND (u.display_name LIKE ? OR p.academy_club LIKE ? OR (p.bio LIKE ?` + minorGuard + `))`
		args = append(args, like, like, like, v, v)
	}
	if f.Role != "" {
		s += ` AND EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role = ?)`
		args = append(args, f.Role)
	}
	if f.SportID > 0 {
		s += ` AND p.sport_id = ?`
		args = append(args, f.SportID)
	}
	if f.PositionID > 0 {
		s += ` AND p.position_id = ?`
		args = append(args, f.PositionID)
	}
	if f.State != "" {
		s += ` AND p.state = ?`
		args = append(args, f.State)
	}
	if f.District != "" {
		s += ` AND p.district = ?` + minorGuard
		args = append(args, f.District, v, v)
	}
	if f.City != "" {
		s += ` AND p.city = ?` + minorGuard
		args = append(args, f.City, v, v)
	}
	if f.AgeMin != nil {
		s += ` AND p.birth_year IS NOT NULL AND p.birth_year <= ?`
		args = append(args, pq.Year-*f.AgeMin)
	}
	if f.AgeMax != nil {
		s += ` AND p.birth_year IS NOT NULL AND p.birth_year >= ?`
		args = append(args, pq.Year-*f.AgeMax)
	}
	if f.AchievementLevel != "" {
		s += ` AND EXISTS (SELECT 1 FROM achievements a WHERE a.user_id = u.id AND a.level = ?)`
		args = append(args, f.AchievementLevel)
	}
	s += `) t`
	if pq.Cursor.ID > 0 {
		b := 0
		if pq.Cursor.Time == "1" {
			b = 1
		}
		s += ` WHERE (t.boosted < ? OR (t.boosted = ? AND t.id < ?))`
		args = append(args, b, b, pq.Cursor.ID)
	}
	s += ` ORDER BY t.boosted DESC, t.id DESC LIMIT ?`
	args = append(args, pq.Limit)

	rows, err := r.db.QueryContext(ctx, s, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []personRow{}
	for rows.Next() {
		var x personRow
		var bio sql.NullString
		if err := rows.Scan(&x.ID, &bio, &x.FollowersCount, &x.IsMinor, &x.Boosted); err != nil {
			return nil, err
		}
		if bio.Valid {
			x.Bio = &bio.String
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

const eventsSQL = `SELECT e.id, e.title, s.id, s.code, s.name, e.event_type, e.participant_type, e.competition_level,
	  e.gender_category, e.age_min, e.age_max, e.venue_name, e.city, e.district, e.state, e.starts_at, e.ends_at,
	  e.registration_deadline, e.entry_fee_paise, e.max_slots, e.registrations_count, e.banner_upload_id, e.status,
	  e.featured_at, e.organiser_id
	FROM events e
	JOIN sports s ON s.id = e.sport_id
	JOIN users ou ON ou.id = e.organiser_id AND ou.status = 'active'
	WHERE e.status = 'published' AND (e.title LIKE ? OR e.venue_name LIKE ? OR e.city LIKE ?)
	  AND NOT EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = ? AND bl.blocked_id = e.organiser_id) OR (bl.blocker_id = e.organiser_id AND bl.blocked_id = ?))
	  AND `

const eventsOrderSQL = `
	ORDER BY e.starts_at ASC, e.id ASC LIMIT ?`

// events searches published events. elCond / elArgs are the viewer's location-eligibility
// condition from d.Eligibility.FeedFilter (alias "e"; built by the platform with bind params).
func (r *repo) events(ctx context.Context, viewer int64, q string, elCond string, elArgs []any, limit int) ([]eventRow, error) {
	like := likePattern(q)
	args := append([]any{like, like, like, viewer, viewer}, elArgs...)
	rows, err := r.db.QueryContext(ctx, eventsSQL+elCond+eventsOrderSQL, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []eventRow{}
	for rows.Next() {
		var x eventRow
		var ageMin, ageMax, banner sql.NullInt64
		var district sql.NullString
		var starts, ends, deadline time.Time
		var featured sql.NullTime
		if err := rows.Scan(&x.ID, &x.Title, &x.Sport.ID, &x.Sport.Code, &x.Sport.Name, &x.EventType, &x.ParticipantType,
			&x.CompetitionLevel, &x.GenderCategory, &ageMin, &ageMax, &x.VenueName, &x.City, &district, &x.State,
			&starts, &ends, &deadline, &x.EntryFeePaise, &x.MaxSlots, &x.RegistrationsCount, &banner, &x.Status,
			&featured, &x.OrganiserID); err != nil {
			return nil, err
		}
		x.AgeMin, x.AgeMax = intPtr(ageMin), intPtr(ageMax)
		if district.Valid {
			x.District = &district.String
		}
		if banner.Valid {
			b := banner.Int64
			x.BannerID = &b
		}
		x.StartsAt, x.EndsAt, x.RegistrationDeadline = ts(starts), ts(ends), ts(deadline)
		x.SlotsLeft = max(0, x.MaxSlots-x.RegistrationsCount)
		x.IsFeatured = featured.Valid
		out = append(out, x)
	}
	return out, rows.Err()
}

const communitiesSQL = `SELECT c.id, c.title, c.description, c.avatar_upload_id, s.id, s.code, s.name, c.city, c.state,
	  c.visibility, c.join_policy, c.member_count, c.owner_id, c.created_at, cm.status, cm.role
	FROM conversations c
	LEFT JOIN sports s ON s.id = c.sport_id
	LEFT JOIN conversation_members cm ON cm.conversation_id = c.id AND cm.user_id = ?
	WHERE c.type = 'community' AND c.visibility = 'public' AND c.status = 'active'
	  AND (c.title LIKE ? OR c.description LIKE ?)
	  AND NOT EXISTS (SELECT 1 FROM blocks bl WHERE (bl.blocker_id = ? AND bl.blocked_id = c.owner_id) OR (bl.blocker_id = c.owner_id AND bl.blocked_id = ?))
	ORDER BY c.member_count DESC, c.id DESC LIMIT ?`

func (r *repo) communities(ctx context.Context, viewer int64, q string, limit int) ([]communityRow, error) {
	like := likePattern(q)
	rows, err := r.db.QueryContext(ctx, communitiesSQL, viewer, like, like, viewer, viewer, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []communityRow{}
	for rows.Next() {
		var x communityRow
		var title, desc, sportCode, sportName, city, state, myStatus, myRole sql.NullString
		var avatar, sportID, owner sql.NullInt64
		var created time.Time
		if err := rows.Scan(&x.ID, &title, &desc, &avatar, &sportID, &sportCode, &sportName, &city, &state,
			&x.Visibility, &x.JoinPolicy, &x.MemberCount, &owner, &created, &myStatus, &myRole); err != nil {
			return nil, err
		}
		x.Title = title.String
		x.Description, x.City, x.State = strPtr(desc), strPtr(city), strPtr(state)
		if avatar.Valid {
			a := avatar.Int64
			x.AvatarID = &a
		}
		if sportID.Valid {
			x.Sport = &SportRef{ID: sportID.Int64, Code: sportCode.String, Name: sportName.String}
		}
		if owner.Valid {
			o := owner.Int64
			x.OwnerID = &o
		}
		x.CreatedAt = ts(created)
		x.MyStatus = "none"
		switch myStatus.String {
		case "active", "pending", "invited", "banned":
			x.MyStatus = myStatus.String
		}
		if x.MyStatus == "active" && myRole.Valid {
			role := myRole.String
			x.MyRole = &role
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// likePattern escapes LIKE wildcards in the user's term and wraps it in %...%.
// The pattern is always a bind argument, never part of the SQL text.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

func intPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
