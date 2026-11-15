package locationidentity

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	me      = int64(42)
	adminID = int64(1)

	stateTN    = int64(23)
	thanjavur  = int64(26)
	coimbatore = int64(4)
)

var now = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func decode(t *testing.T, body []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return e
}

func newDeps(t *testing.T, cfg map[string]any) (*app.Deps, sqlmock.Sqlmock) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return now }
	return d, mock
}

func userSetup(t *testing.T, cfg map[string]any, u userdir.Basic) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := newDeps(t, cfg)
	r, routes := testutil.UserEngine(u)
	New(d).Routes(routes)
	return d, mock, r
}

func adminSetup(t *testing.T, cfg map[string]any, perms ...string) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := newDeps(t, cfg)
	r, routes := testutil.AdminEngine(adminID, perms)
	New(d).Routes(routes)
	return d, mock, r
}

// recorder captures published events of one type (the test bus is synchronous).
type recorder struct {
	mu  sync.Mutex
	got []events.LocationIdentityPayload
}

func record(d *app.Deps, typ string) *recorder {
	rec := &recorder{}
	d.Dispatcher.On(typ, func(_ context.Context, e events.Event) error {
		var p events.LocationIdentityPayload
		_ = e.Decode(&p)
		rec.mu.Lock()
		rec.got = append(rec.got, p)
		rec.mu.Unlock()
		return nil
	})
	return rec
}

// ---- row fixtures ----

var identityColNames = []string{"id", "user_id", "identity_type", "district_id", "dname", "state_id", "scode", "sname", "is_ut",
	"proof_type", "proof_holder", "proof_upload_id", "aadhaar_last4", "notice_version", "status",
	"rejection_code", "rejection_reason", "submitted_at", "reviewed_at", "reviewed_by", "verified_at",
	"expires_at", "proof_purged_at", "updated_at"}

type fx struct {
	ID         int64
	User       int64
	Type       string
	District   int64
	Proof      string
	Status     string
	Upload     any
	VerifiedAt any
	ExpiresAt  any
	Note       any // review_note
}

func (f fx) values() []driver.Value {
	name := "Thanjavur"
	if f.District == coimbatore {
		name = "Coimbatore"
	}
	if f.User == 0 {
		f.User = me
	}
	if f.Proof == "" {
		f.Proof = "rent_agreement"
	}
	return []driver.Value{f.ID, f.User, f.Type, f.District, name, stateTN, "TN", "Tamil Nadu", false,
		f.Proof, "self", f.Upload, nil, 1, f.Status,
		nil, nil, now.Add(-48 * time.Hour), nil, nil, f.VerifiedAt,
		f.ExpiresAt, nil, now.Add(-time.Hour)}
}

func identityRows(fs ...fx) *sqlmock.Rows {
	rows := sqlmock.NewRows(identityColNames)
	for _, f := range fs {
		rows.AddRow(f.values()...)
	}
	return rows
}

func detailRows(f fx) *sqlmock.Rows {
	cols := append(append([]string{}, identityColNames...), "is_minor", "review_note", "pdistrict", "aname", "content_type")
	vals := append(f.values(), false, f.Note, "Thanjavur", nil, "image/jpeg")
	return sqlmock.NewRows(cols).AddRow(vals...)
}

func summaryRows(ids ...int64) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "name", "role", "verification", "photo", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"})
	for _, id := range ids {
		rows.AddRow(id, "Asha K", "player", "verified", nil, "Thanjavur", "Tamil Nadu", 3, "football", "Football", nil, nil, nil)
	}
	return rows
}

const summariesRe = `WHERE u.status <> 'deleted' AND u.id IN`

func noticeRows(id int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "version", "title"}).AddRow(id, 2, "Location identity notice")
}

func districtStateRows(id int64, active bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "state_id", "name", "active", "code", "sname", "is_ut"}).
		AddRow(id, stateTN, "Thanjavur", active, "TN", "Tamil Nadu", false)
}

func openRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "district_id", "status", "verified_at"})
}

func reviewRows(id int64, typ, proof, status string, upload any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "identity_type", "district_id", "proof_type", "proof_upload_id", "status"}).
		AddRow(id, me, typ, thanjavur, proof, upload, status)
}

func changedRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "user_id", "identity_type", "district_id", "status", "expires_at"})
}

// expectPurge is media.Purge for an upload that still has its object.
func expectPurge(mock sqlmock.Sqlmock, uploadID int64, key string) {
	mock.ExpectQuery(`SELECT s3_key, status FROM media_uploads WHERE id = \?`).WithArgs(uploadID).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}).AddRow(key, "attached"))
	mock.ExpectExec(`UPDATE media_uploads SET status = 'deleted' WHERE id = \?`).WithArgs(uploadID).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectMarkPurged(mock sqlmock.Sqlmock, id, uploadID int64) {
	mock.ExpectExec(`UPDATE user_location_identities SET proof_purged_at = \?, proof_upload_id = NULL WHERE id = \? AND proof_upload_id = \?`).
		WithArgs(now, id, uploadID).WillReturnResult(sqlmock.NewResult(0, 1))
}

// expectDetail is Detail(sign=false): row, history, user summary.
func expectDetail(mock sqlmock.Sqlmock, f fx) {
	mock.ExpectQuery(`SELECT l.id[\s\S]*u.is_minor, l.review_note, p.district, a.name, m.content_type[\s\S]*WHERE l.id = \?`).WithArgs(f.ID).WillReturnRows(detailRows(f))
	mock.ExpectQuery(`WHERE l.user_id = \? AND l.identity_type = \? AND l.id < \? ORDER BY l.id DESC LIMIT \?`).
		WithArgs(me, f.Type, f.ID, historyLimit).WillReturnRows(identityRows())
	mock.ExpectQuery(summariesRe).WithArgs(me).WillReturnRows(summaryRows(me))
}

func strp(s string) *string { return &s }
