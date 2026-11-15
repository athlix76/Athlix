// Package userdir is the shared, read-only view of users that every module may use:
// auth status checks, public user summaries, follow and block lookups.
//
// Writes to users/profiles/follows/blocks happen ONLY in their owning modules
// (auth, user, profile, follow). After a write that changes Basic fields, call Invalidate.
package userdir

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"athlix/backend/internal/platform/db"
)

// Basic is the cached core of a user row.
type Basic struct {
	ID                    int64
	DisplayName           string
	PrimaryRole           string   // "" until chosen
	Roles                 []string // player | coach | organiser
	Status                string   // active | suspended | deleted
	OnboardingStep        string   // role | verification | guardian | profile | done
	VerificationStatus    string
	IsMinor               bool
	GuardianConsentStatus string
}

// HasRole reports whether the user holds role.
func (b Basic) HasRole(role string) bool {
	for _, r := range b.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Summary is the public card shown anywhere a user appears (docs/api.md "UserSummary").
type Summary struct {
	ID          int64     `json:"id"`
	DisplayName string    `json:"displayName"`
	PhotoURL    *string   `json:"photoUrl"`
	PrimaryRole string    `json:"primaryRole"`
	Sport       *SportRef `json:"sport"`
	Position    *SportRef `json:"position"`
	City        *string   `json:"city"`
	State       *string   `json:"state"`
	IsVerified  bool      `json:"isVerified"`
}

// SportRef is {id, code, name} (docs/api.md "SportRef"). For a position, code is the
// sport_positions.code.
type SportRef struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// PhotoSigner turns a media upload id into a short-lived view URL ("" if unavailable).
type PhotoSigner interface {
	ViewURLs(ctx context.Context, uploadIDs []int64) map[int64]string
}

// Directory reads users with a tiny per-process cache.
type Directory struct {
	db     *sql.DB
	photos PhotoSigner
	ttl    time.Duration
	mu     sync.Mutex
	cache  map[int64]cached
}

type cached struct {
	b  Basic
	at time.Time
}

// New builds a Directory. photos may be nil (no photo URLs).
func New(d *sql.DB, photos PhotoSigner) *Directory {
	return &Directory{db: d, photos: photos, ttl: 10 * time.Second, cache: map[int64]cached{}}
}

// SetPhotoSigner wires the media service after construction (avoids an init cycle).
func (d *Directory) SetPhotoSigner(p PhotoSigner) { d.photos = p }

// Get returns Basic for id (sql.ErrNoRows if missing).
func (d *Directory) Get(ctx context.Context, id int64) (Basic, error) {
	d.mu.Lock()
	if c, ok := d.cache[id]; ok && time.Since(c.at) < d.ttl {
		d.mu.Unlock()
		return c.b, nil
	}
	d.mu.Unlock()
	var b Basic
	var name, role sql.NullString
	err := d.db.QueryRowContext(ctx, `SELECT id, display_name, primary_role, status, onboarding_step, verification_status, is_minor, guardian_consent_status FROM users WHERE id = ?`, id).
		Scan(&b.ID, &name, &role, &b.Status, &b.OnboardingStep, &b.VerificationStatus, &b.IsMinor, &b.GuardianConsentStatus)
	if err != nil {
		return Basic{}, err
	}
	b.DisplayName, b.PrimaryRole = name.String, role.String
	rows, err := d.db.QueryContext(ctx, `SELECT role FROM user_roles WHERE user_id = ?`, id)
	if err != nil {
		return Basic{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return Basic{}, err
		}
		b.Roles = append(b.Roles, r)
	}
	if err := rows.Err(); err != nil {
		return Basic{}, err
	}
	d.mu.Lock()
	d.cache[id] = cached{b: b, at: time.Now()}
	d.mu.Unlock()
	return b, nil
}

// Invalidate drops id from this process's cache (call after changing users/user_roles).
func (d *Directory) Invalidate(id int64) {
	d.mu.Lock()
	delete(d.cache, id)
	d.mu.Unlock()
}

// Summaries returns public cards for ids (missing/deleted users are omitted).
func (d *Directory) Summaries(ctx context.Context, ids []int64) (map[int64]Summary, error) {
	out := map[int64]Summary{}
	if len(ids) == 0 {
		return out, nil
	}
	q := `SELECT u.id, COALESCE(u.display_name,''), COALESCE(u.primary_role,''), u.verification_status,
	             p.photo_upload_id, p.city, p.state, s.id, s.code, s.name, sp.id, sp.code, sp.name
	      FROM users u
	      LEFT JOIN profiles p ON p.user_id = u.id
	      LEFT JOIN sports s ON s.id = p.sport_id
	      LEFT JOIN sport_positions sp ON sp.id = p.position_id
	      WHERE u.status <> 'deleted' AND u.id IN (` + db.Placeholders(len(ids)) + `)`
	rows, err := d.db.QueryContext(ctx, q, db.Int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photoOf := map[int64]int64{}
	for rows.Next() {
		var s Summary
		var verification string
		var photo, sportID, posID sql.NullInt64
		var city, state, sportCode, sportName, posCode, posName sql.NullString
		if err := rows.Scan(&s.ID, &s.DisplayName, &s.PrimaryRole, &verification, &photo, &city, &state, &sportID, &sportCode, &sportName, &posID, &posCode, &posName); err != nil {
			return nil, err
		}
		s.IsVerified = verification == "verified"
		if city.Valid {
			s.City = &city.String
		}
		if state.Valid {
			s.State = &state.String
		}
		if sportID.Valid {
			s.Sport = &SportRef{ID: sportID.Int64, Code: sportCode.String, Name: sportName.String}
		}
		if posID.Valid {
			s.Position = &SportRef{ID: posID.Int64, Code: posCode.String, Name: posName.String}
		}
		if photo.Valid {
			photoOf[s.ID] = photo.Int64
		}
		out[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if d.photos != nil && len(photoOf) > 0 {
		pids := make([]int64, 0, len(photoOf))
		for _, p := range photoOf {
			pids = append(pids, p)
		}
		urls := d.photos.ViewURLs(ctx, pids)
		for uid, pid := range photoOf {
			if u, ok := urls[pid]; ok && u != "" {
				s := out[uid]
				s.PhotoURL = &u
				out[uid] = s
			}
		}
	}
	return out, nil
}

// Summary returns one card.
func (d *Directory) Summary(ctx context.Context, id int64) (Summary, bool, error) {
	m, err := d.Summaries(ctx, []int64{id})
	if err != nil {
		return Summary{}, false, err
	}
	s, ok := m[id]
	return s, ok, nil
}

// BlockedEitherWay reports whether a blocked b or b blocked a.
func (d *Directory) BlockedEitherWay(ctx context.Context, a, b int64) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)`, a, b, b, a).Scan(&n)
	return n > 0, err
}

// Follows reports whether follower follows followee.
func (d *Directory) Follows(ctx context.Context, follower, followee int64) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM follows WHERE follower_id = ? AND followee_id = ?`, follower, followee).Scan(&n)
	return n > 0, err
}

// MutualFollow reports whether a and b follow each other.
func (d *Directory) MutualFollow(ctx context.Context, a, b int64) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM follows WHERE (follower_id = ? AND followee_id = ?) OR (follower_id = ? AND followee_id = ?)`, a, b, b, a).Scan(&n)
	return n == 2, err
}

// AdminActive implements authn.AdminChecker.
func (d *Directory) AdminActive(ctx context.Context, adminID int64) (bool, error) {
	var status string
	err := d.db.QueryRowContext(ctx, `SELECT status FROM admin_users WHERE id = ?`, adminID).Scan(&status)
	if db.IsNoRows(err) {
		return false, nil
	}
	return status == "active", err
}
