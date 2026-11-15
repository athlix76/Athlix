// Package usertest holds sqlmock helpers for the queries of user.LoadMe and user.Advance,
// so tests of modules that return Me (auth, verification, guardian, profile) stay short.
package usertest

import (
	"database/sql/driver"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// MeRow describes the user row LoadMe reads. Zero values get sensible defaults.
type MeRow struct {
	ID          int64
	Last4       string // default "3210"
	DisplayName any    // string or nil
	PrimaryRole any    // string or nil
	Status      string // default "active"
	Step        string // default "done"
	Verify      string // default "verified"
	IsMinor     bool
	Guardian    string // default "not_required"
	Roles       []string
	PhotoID     any // int64 or nil; when set, the media view query is expected too
	PhotoKey    string
	Pending     [][]driver.Value // rows of (id, doc_type, version, title)
}

func (r *MeRow) defaults() {
	if r.Last4 == "" {
		r.Last4 = "3210"
	}
	if r.Status == "" {
		r.Status = "active"
	}
	if r.Step == "" {
		r.Step = "done"
	}
	if r.Verify == "" {
		r.Verify = "verified"
	}
	if r.Guardian == "" {
		r.Guardian = "not_required"
	}
}

// ExpectMe expects the queries of user.LoadMe for r.
func ExpectMe(mock sqlmock.Sqlmock, r MeRow) {
	r.defaults()
	mock.ExpectQuery(`SELECT u.id, u.mobile_last4, .* FROM users u LEFT JOIN profiles p ON p.user_id = u.id WHERE u.id = \? AND u.status <> 'deleted'`).
		WithArgs(r.ID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "mobile_last4", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status", "created_at", "photo_upload_id"}).
			AddRow(r.ID, r.Last4, r.DisplayName, r.PrimaryRole, r.Status, r.Step, r.Verify, r.IsMinor, r.Guardian, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), r.PhotoID))
	ExpectRoles(mock, r.ID, r.Roles...)
	pending := sqlmock.NewRows([]string{"id", "doc_type", "version", "title"})
	for _, p := range r.Pending {
		pending.AddRow(p...)
	}
	args := []driver.Value{"terms", "privacy", "community_guidelines"}
	for _, role := range r.Roles {
		if role == "organiser" {
			args = append(args, "organiser_terms")
		}
	}
	mock.ExpectQuery(`SELECT d.id, d.doc_type, d.version, d.title FROM policy_documents d`).WithArgs(append(args, r.ID)...).WillReturnRows(pending)
	if r.PhotoID != nil {
		mock.ExpectQuery(`SELECT id, s3_key FROM media_uploads WHERE status IN`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "s3_key"}).AddRow(r.PhotoID, r.PhotoKey))
	}
}

// ExpectRoles expects the ordered roles query.
func ExpectRoles(mock sqlmock.Sqlmock, userID int64, roles ...string) {
	rows := sqlmock.NewRows([]string{"role"})
	for _, r := range roles {
		rows.AddRow(r)
	}
	mock.ExpectQuery(`SELECT role FROM user_roles WHERE user_id = \? ORDER BY role`).WithArgs(userID).WillReturnRows(rows)
}

// AdvanceRow is the state user.Advance reads.
type AdvanceRow struct {
	PrimaryRole any // string or nil
	Step        string
	Verify      string
	IsMinor     bool
	Guardian    string
	Profiles    int
	Roles       []string
}

// ExpectAdvance expects user.Advance for userID. When next differs from row.Step (and the
// step is not done) the UPDATE of onboarding_step is expected too.
func ExpectAdvance(mock sqlmock.Sqlmock, userID int64, row AdvanceRow, next string) {
	mock.ExpectQuery(`SELECT u.primary_role, u.onboarding_step, .* FROM users u WHERE u.id = \? AND u.status <> 'deleted' FOR UPDATE`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"primary_role", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status", "profiles"}).
			AddRow(row.PrimaryRole, row.Step, row.Verify, row.IsMinor, row.Guardian, row.Profiles))
	if row.Step == "done" {
		return
	}
	ExpectRoles(mock, userID, row.Roles...)
	if next != row.Step {
		mock.ExpectExec(`UPDATE users SET onboarding_step = \? WHERE id = \?`).WithArgs(next, userID).WillReturnResult(sqlmock.NewResult(0, 1))
	}
}
