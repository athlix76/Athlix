package chat

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

// SQL fragments (regexp, matched in order).
const (
	qMembership = `SELECT c\.id, c\.type, c\.status, c\.direct_key, m\.role`
	qConv       = `FROM conversation_members m\s+JOIN conversations c`
	qByClient   = `WHERE x\.sender_id = \? AND x\.client_msg_id = \?`
	qMsgsByID   = `FROM messages x WHERE x\.id IN`
	qMsgsPage   = `FROM messages x WHERE x\.conversation_id = \?`
	qInConv     = `SELECT id FROM messages WHERE id = \? AND conversation_id = \?`
	qBlocks     = `FROM blocks`
	qInsertMsg  = `INSERT INTO messages`
	qTouch      = `UPDATE conversations SET last_message_id`
	qMarkRead   = `UPDATE conversation_members SET last_read_message_id`
	qSummaries  = `FROM users u\s+LEFT JOIN profiles p`
	qMediaView  = `SELECT id, s3_key FROM media_uploads`
	qMembers    = `SELECT user_id FROM conversation_members WHERE conversation_id = \? AND status = 'active'`
	qUser       = `FROM users WHERE id = \?`
	qRoles      = `FROM user_roles`
	qPrivacy    = `FROM user_settings`
	qFollows    = `FROM follows WHERE follower_id = \? AND followee_id = \?`
	qMutual     = `FROM follows WHERE \(follower_id`
	qConsult    = `FROM consultation_requests`
	qDeletable  = `SELECT x\.conversation_id, x\.sender_id, x\.status, c\.type, m\.role`
	qLockGroup  = `SELECT c\.id, c\.status, c\.member_count, m\.status`
)

var (
	msgColumns  = []string{"id", "conversation_id", "sender_id", "kind", "body", "media_upload_id", "reply_to_id", "client_msg_id", "status", "created_at"}
	convColumns = []string{"id", "type", "title", "avatar_upload_id", "event_id", "member_count", "last_message_id", "status", "activity_at", "direct_key", "role", "muted_until", "unread"}
	userColumns = []string{"id", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status"}
	testNow     = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
)

// fakeDelivery records frames per connection; connections in gone return ErrGone.
type fakeDelivery struct {
	mu   sync.Mutex
	got  map[string][][]byte
	gone map[string]bool
}

func newFake() *fakeDelivery {
	return &fakeDelivery{got: map[string][][]byte{}, gone: map[string]bool{}}
}

func (f *fakeDelivery) Send(_ context.Context, conn string, b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone[conn] {
		return ErrGone
	}
	f.got[conn] = append(f.got[conn], append([]byte(nil), b...))
	return nil
}

// types lists the frame types delivered to conn.
func (f *fakeDelivery) types(conn string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, b := range f.got[conn] {
		var fr struct{ Type string }
		_ = json.Unmarshal(b, &fr)
		out = append(out, fr.Type)
	}
	return out
}

type env struct {
	t    *testing.T
	d    *app.Deps
	mock sqlmock.Sqlmock
	m    *Module
	s    *Service
	fd   *fakeDelivery

	subscribed bool
}

// newEnv builds the module on test deps with a fake delivery.
func newEnv(t *testing.T, cfg map[string]any) *env {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	d.Now = func() time.Time { return testNow }
	m := New(d)
	fd := newFake()
	m.Service().SetDelivery(fd)
	return &env{t: t, d: d, mock: mock, m: m, s: m.Service(), fd: fd}
}

func (e *env) userEngine(u userdir.Basic) *gin.Engine {
	r, routes := testutil.UserEngine(u)
	e.m.Routes(routes)
	return r
}

func (e *env) adminEngine(id int64, perms []string) *gin.Engine {
	r, routes := testutil.AdminEngine(id, perms)
	e.m.Routes(routes)
	return r
}

func (e *env) connect(connID string, userID int64) {
	e.t.Helper()
	if err := e.s.RegisterConnection(context.Background(), connID, userID); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) expectMember(convID, userID int64, typ, status string, directKey any, role string) {
	e.mock.ExpectQuery(qMembership).WithArgs(convID, userID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "type", "status", "direct_key", "role"}).AddRow(convID, typ, status, directKey, role))
}

func (e *env) expectNotMember(convID, userID int64) {
	e.mock.ExpectQuery(qMembership).WithArgs(convID, userID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "type", "status", "direct_key", "role"}))
}

func (e *env) expectNoClientMsg(userID int64, cid string) {
	e.mock.ExpectQuery(qByClient).WithArgs(userID, cid).WillReturnRows(sqlmock.NewRows(msgColumns))
}

// expectSendTx expects the insert transaction for a text message.
func (e *env) expectSendTx(convID, userID, msgID int64, body, cid string) {
	e.mock.ExpectBegin()
	e.mock.ExpectExec(qInsertMsg).WithArgs(convID, userID, "text", body, nil, nil, cid, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(msgID, 1))
	e.mock.ExpectExec(qTouch).WithArgs(msgID, sqlmock.AnyArg(), convID).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(qMarkRead).WithArgs(msgID, convID, userID).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectCommit()
}

func (e *env) expectSummaries(ids ...int64) {
	r := sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "s.id", "s.code", "s.name", "sp.id", "sp.code", "sp.name"})
	for _, id := range ids {
		r.AddRow(id, "User", "player", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}
	e.mock.ExpectQuery(qSummaries).WillReturnRows(r)
}

func (e *env) expectActiveMembers(convID int64, ids ...int64) {
	r := sqlmock.NewRows([]string{"user_id"})
	for _, id := range ids {
		r.AddRow(id)
	}
	e.mock.ExpectQuery(qMembers).WithArgs(convID).WillReturnRows(r)
}

func (e *env) expectUser(b userdir.Basic) {
	e.mock.ExpectQuery(qUser).WithArgs(b.ID).WillReturnRows(sqlmock.NewRows(userColumns).
		AddRow(b.ID, b.DisplayName, b.PrimaryRole, b.Status, b.OnboardingStep, b.VerificationStatus, b.IsMinor, b.GuardianConsentStatus))
	e.mock.ExpectQuery(qRoles).WithArgs(b.ID).WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow(b.PrimaryRole))
}

func (e *env) expectCount(q string, n int, args ...any) {
	ex := e.mock.ExpectQuery(q)
	if len(args) > 0 {
		ex = ex.WithArgs(toDriver(args)...)
	}
	ex.WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(n))
}

func msgRowOf(id, convID, sender int64, body, cid, status string) []any {
	return []any{id, convID, sender, "text", body, nil, nil, cid, status, testNow}
}

func row(cols []string, vals ...[]any) *sqlmock.Rows {
	r := sqlmock.NewRows(cols)
	for _, v := range vals {
		r.AddRow(toDriver(v)...)
	}
	return r
}

func toDriver(v []any) []driver.Value {
	out := make([]driver.Value, len(v))
	for i, x := range v {
		out[i] = x
	}
	return out
}

// body decodes a JSON response.
func body(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return m
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	e, _ := body(t, w)["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func data(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	d, _ := body(t, w)["data"].(map[string]any)
	return d
}

func adult(id int64) userdir.Basic { return testutil.VerifiedUser(id) }

func minor(id int64) userdir.Basic {
	u := testutil.VerifiedUser(id)
	u.IsMinor = true
	u.GuardianConsentStatus = "approved"
	return u
}
