package organiser

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
	"athlix/backend/internal/testutil"
)

const (
	me      = int64(7)
	adminID = int64(3)
)

var (
	fixedNow  = time.Date(2026, 6, 15, 6, 0, 0, 0, time.UTC)
	submitted = time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
)

func q(s string) string { return regexp.QuoteMeta(s) }

type body struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func status(t *testing.T, w *httptest.ResponseRecorder, want int) body {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body)
	}
	var b body
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

type env struct {
	d      *app.Deps
	mock   sqlmock.Sqlmock
	m      *Module
	user   *gin.Engine
	admin  *gin.Engine
	events []events.Event
}

func organiser() userdir.Basic {
	u := testutil.VerifiedUser(me)
	u.PrimaryRole, u.Roles = "organiser", []string{"organiser"}
	return u
}

func newEnvAs(t *testing.T, u userdir.Basic, perms ...string) *env {
	t.Helper()
	d, mock := testutil.Deps(t, nil)
	d.Now = func() time.Time { return fixedNow }
	e := &env{d: d, mock: mock, m: New(d)}
	for _, name := range []string{events.OrganiserApproved, events.OrganiserRejected} {
		d.Dispatcher.On(name, func(_ context.Context, ev events.Event) error {
			e.events = append(e.events, ev)
			return nil
		})
	}
	r, routes := testutil.UserEngine(u)
	e.m.Routes(routes)
	e.user = r
	if perms == nil {
		perms = []string{"organisers.review"}
	}
	a, aroutes := testutil.AdminEngine(adminID, perms)
	e.m.Routes(aroutes)
	e.admin = a
	return e
}

func newEnv(t *testing.T) *env { return newEnvAs(t, organiser()) }

func (e *env) do(method, path string, in any) *httptest.ResponseRecorder {
	return testutil.Request(e.user, method, "/v1"+path, in, nil)
}

func (e *env) doAdmin(method, path string, in any) *httptest.ResponseRecorder {
	return testutil.Request(e.admin, method, "/v1/admin"+path, in, nil)
}

func decodeV(t *testing.T, b body) Verification {
	t.Helper()
	var v Verification
	if err := json.Unmarshal(b.Data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// ---- expectations ----

type prow struct {
	name, typ, status, note any
	submittedAt, reviewed   any
}

func (e *env) expectProfile(uid int64, p *prow, forUpdate bool) {
	query := q(profileCols)
	if forUpdate {
		query += q(" FOR UPDATE")
	} else {
		query += "$"
	}
	rows := sqlmock.NewRows([]string{"org_name", "org_type", "status", "review_note", "submitted_at", "reviewed_at"})
	if p != nil {
		rows.AddRow(p.name, p.typ, p.status, p.note, p.submittedAt, p.reviewed)
	}
	e.mock.ExpectQuery(query).WithArgs(uid).WillReturnRows(rows)
}

// srow is one step joined with a submission (subID nil = not filled).
type srow struct {
	id                       int64
	code, typ                string
	required, active         bool
	subID, value, upload     any
	subStatus, note, descrip any
}

var (
	stepText  = srow{id: 1, code: "org_details", typ: "text", required: true, active: true}
	stepProof = srow{id: 2, code: "past_event_proof", typ: "document", active: true}
	stepAffil = srow{id: 3, code: "affiliation_doc", typ: "document", active: true}
	stepTerms = srow{id: 4, code: "accept_terms", typ: "checkbox", required: true, active: true}
)

func filled(s srow, value, upload any) srow {
	s.subID, s.value, s.upload, s.subStatus = s.id*10, value, upload, "submitted"
	return s
}

func (e *env) expectSteps(uid int64, steps ...srow) {
	rows := sqlmock.NewRows([]string{"id", "step_code", "title", "description", "input_type", "is_required", "active", "vid", "value_text", "document_upload_id", "status", "review_note"})
	for _, s := range steps {
		rows.AddRow(s.id, s.code, "Title "+s.code, s.descrip, s.typ, s.required, s.active, s.subID, s.value, s.upload, s.subStatus, s.note)
	}
	e.mock.ExpectQuery(q(`SELECT s.id, s.step_code, s.title, s.description, s.input_type, s.is_required, s.active, v.id`)).WithArgs(uid).WillReturnRows(rows)
}

func (e *env) expectViewURLs(ids ...int64) {
	args := make([]driver.Value, len(ids))
	rows := sqlmock.NewRows([]string{"id", "s3_key"})
	for i, id := range ids {
		args[i] = id
		rows.AddRow(id, "organiser_document/7/doc.pdf")
	}
	e.mock.ExpectQuery(q(`SELECT id, s3_key FROM media_uploads WHERE status IN ('confirmed','attached')`)).WithArgs(args...).WillReturnRows(rows)
}

func (e *env) expectActiveSteps() {
	e.mock.ExpectQuery(q(`SELECT id, input_type FROM organiser_verification_steps WHERE active = 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "input_type"}).AddRow(1, "text").AddRow(2, "document").AddRow(3, "document").AddRow(4, "checkbox"))
}

func (e *env) expectDocuments(docs map[int64]int64) {
	rows := sqlmock.NewRows([]string{"step_id", "document_upload_id"})
	for step, up := range docs {
		rows.AddRow(step, up)
	}
	e.mock.ExpectQuery(q(`SELECT step_id, document_upload_id FROM organiser_verification_submissions WHERE user_id = ? AND document_upload_id IS NOT NULL FOR UPDATE`)).WithArgs(me).WillReturnRows(rows)
}

func (e *env) expectSaveProfile(name, typ any) {
	e.mock.ExpectExec(q(`INSERT INTO organiser_profiles (user_id, org_name, org_type, status) VALUES (?, ?, ?, 'draft') ON DUPLICATE KEY UPDATE`)).
		WithArgs(me, name, typ).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectUpsert(step int64, value, upload any) {
	e.mock.ExpectExec(q(`INSERT INTO organiser_verification_submissions (user_id, step_id, value_text, document_upload_id, status) VALUES (?, ?, ?, ?, 'submitted') ON DUPLICATE KEY UPDATE`)).
		WithArgs(me, step, value, upload).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectDeleteSub(step int64) {
	e.mock.ExpectExec(q(`DELETE FROM organiser_verification_submissions WHERE user_id = ? AND step_id = ?`)).WithArgs(me, step).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectUpload(id, owner int64, purpose, status string) {
	rows := sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"})
	if purpose != "" {
		rows.AddRow(id, owner, purpose, purpose+"/7/x.pdf", "application/pdf", 100, status)
	}
	e.mock.ExpectQuery(q(`SELECT id, user_id, purpose, s3_key, content_type, size_bytes, status FROM media_uploads WHERE id = ? AND user_id = ?`)).
		WithArgs(id, owner).WillReturnRows(rows)
}

func (e *env) expectClaim(id int64) {
	e.expectUpload(id, me, "organiser_document", "confirmed")
	e.mock.ExpectExec(q(`UPDATE media_uploads SET status = 'attached'`)).WithArgs(id, me).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectPurge(id int64) {
	e.mock.ExpectQuery(q(`SELECT s3_key, status FROM media_uploads WHERE id = ?`)).WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "status"}).AddRow("organiser_document/7/old.pdf", "attached"))
	e.mock.ExpectExec(q(`UPDATE media_uploads SET status = 'deleted' WHERE id = ?`)).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
}

func (e *env) expectSummary(uid int64) {
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(uid, "Asha", "organiser", "verified", nil, "Pune", "Maharashtra", nil, nil, nil, nil, nil, nil))
}

func (e *env) expectNoSummary(uid int64) {
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(uid).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func (e *env) expectUser(b userdir.Basic) {
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status, onboarding_step, verification_status, is_minor, guardian_consent_status FROM users WHERE id = ?`)).
		WithArgs(b.ID).WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "status", "onboarding_step", "verification_status", "is_minor", "guardian_consent_status"}).
		AddRow(b.ID, b.DisplayName, b.PrimaryRole, b.Status, b.OnboardingStep, b.VerificationStatus, b.IsMinor, b.GuardianConsentStatus))
	rows := sqlmock.NewRows([]string{"role"})
	for _, r := range b.Roles {
		rows.AddRow(r)
	}
	e.mock.ExpectQuery(q(`SELECT role FROM user_roles WHERE user_id = ?`)).WithArgs(b.ID).WillReturnRows(rows)
}

func (e *env) expectAudit(action, target string, details any) {
	e.mock.ExpectExec(q(`INSERT INTO admin_audit_log (admin_id, action, target_type, target_id, details, ip) VALUES`)).
		WithArgs(adminID, action, "user", target, details, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
}

// ---- role ----

func TestNonOrganiserIsForbidden(t *testing.T) {
	e := newEnvAs(t, testutil.VerifiedUser(me)) // a player
	for _, rt := range [][2]string{{http.MethodGet, ""}, {http.MethodPut, ""}, {http.MethodPost, "/submit"}} {
		if b := status(t, e.do(rt[0], "/organiser/verification"+rt[1], map[string]any{}), http.StatusForbidden); b.Error.Code != "FORBIDDEN" {
			t.Fatalf("%v: %+v", rt, b.Error)
		}
	}
}

// ---- get ----

func TestGetNotStarted(t *testing.T) {
	e := newEnv(t)
	e.expectProfile(me, nil, false)
	e.expectSteps(me, stepText, stepProof, stepTerms)
	w := e.do(http.MethodGet, "/organiser/verification", nil)
	v := decodeV(t, status(t, w, http.StatusOK))
	if v.Status != "not_started" || v.OrgName != nil || len(v.Steps) != 3 || v.Steps[0].Submission != nil || !v.Steps[0].IsRequired || v.Steps[1].IsRequired {
		t.Fatalf("v = %+v", v)
	}
	// Optional fields are present as null.
	for _, k := range []string{`"orgName":null`, `"orgType":null`, `"reviewNote":null`, `"submittedAt":null`, `"reviewedAt":null`, `"submission":null`, `"description":null`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("missing %s in %s", k, w.Body)
		}
	}
}

func TestGetFilled(t *testing.T) {
	e := newEnv(t)
	e.expectProfile(me, &prow{name: "Pune Strikers", typ: "club", status: "rejected", note: "Blurry proof", submittedAt: submitted, reviewed: fixedNow}, false)
	old := srow{id: 9, code: "old_step", typ: "text", required: true, active: false}
	proof := filled(stepProof, nil, int64(55))
	proof.subStatus, proof.note = "rejected", "Blurry"
	e.expectSteps(me, filled(stepText, "Pune Strikers Club", nil), proof, filled(stepTerms, "true", nil), filled(old, "x", nil), stepAffil)
	e.expectViewURLs(55)
	v := decodeV(t, status(t, e.do(http.MethodGet, "/organiser/verification", nil), http.StatusOK))
	if v.Status != "rejected" || *v.OrgName != "Pune Strikers" || *v.OrgType != "club" || *v.ReviewNote != "Blurry proof" || !v.SubmittedAt.Equal(submitted) {
		t.Fatalf("v = %+v", v)
	}
	s := v.Steps
	if *s[0].Submission.ValueText != "Pune Strikers Club" || s[0].Submission.Checked != nil || s[0].Submission.Document != nil {
		t.Fatalf("text = %+v", s[0].Submission)
	}
	doc := s[1].Submission
	if doc.Document == nil || doc.Document.UploadID != 55 || doc.Document.URL == nil || !strings.HasPrefix(*doc.Document.URL, "https://storage.test/get/") || doc.ValueText != nil || doc.Status != "rejected" || *doc.ReviewNote != "Blurry" {
		t.Fatalf("doc = %+v", doc)
	}
	if s[2].Submission.Checked == nil || !*s[2].Submission.Checked || s[2].Submission.ValueText != nil {
		t.Fatalf("checkbox = %+v", s[2].Submission)
	}
	if s[3].IsRequired || s[3].Submission == nil {
		t.Fatal("an inactive filled step is shown but never required")
	}
	if s[4].Submission != nil {
		t.Fatal("unfilled step")
	}
}

// ---- save ----

func (e *env) expectGetAfter(p *prow, steps ...srow) {
	e.expectProfile(me, p, false)
	e.expectSteps(me, steps...)
}

func TestSaveHappyPathFromNotStarted(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectBegin()
	e.expectProfile(me, nil, true)
	e.expectActiveSteps()
	e.expectDocuments(nil)
	e.expectSaveProfile("Pune Strikers", "club")
	e.expectUpsert(1, "Pune Strikers Club", nil)
	e.expectClaim(55)
	e.expectUpsert(2, nil, int64(55))
	e.expectUpsert(4, "true", nil)
	e.mock.ExpectCommit()
	e.expectGetAfter(&prow{name: "Pune Strikers", typ: "club", status: "draft"}, filled(stepText, "Pune Strikers Club", nil), filled(stepProof, nil, int64(55)), filled(stepTerms, "true", nil))
	e.expectViewURLs(55)
	in := map[string]any{"orgName": "  Pune Strikers ", "orgType": "club", "steps": []map[string]any{
		{"stepId": 1, "valueText": " Pune Strikers Club "},
		{"stepId": 2, "documentUploadId": 55},
		{"stepId": 4, "checked": true},
	}}
	if v := decodeV(t, status(t, e.do(http.MethodPut, "/organiser/verification", in), http.StatusOK)); v.Status != "draft" || len(v.Steps) != 3 {
		t.Fatalf("v = %+v", v)
	}
}

func TestSaveKeepsMissingFieldsAndClearsSteps(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{name: "Old Name", typ: "academy", status: "rejected", note: "Fix it"}, true)
	e.expectActiveSteps()
	e.expectDocuments(map[int64]int64{2: 50, 3: 51})
	e.expectSaveProfile("Old Name", "academy") // missing fields are unchanged
	e.expectClaim(60)
	e.expectUpsert(2, nil, int64(60)) // replaced: 50 is purged after commit
	e.expectDeleteSub(3)              // removed: 51 is purged after commit
	e.expectDeleteSub(1)              // empty text clears
	e.expectDeleteSub(4)              // unchecked clears
	e.mock.ExpectCommit()
	e.expectPurge(50)
	e.expectPurge(51)
	e.expectGetAfter(&prow{name: "Old Name", typ: "academy", status: "draft", note: "Fix it"}, stepText)
	in := map[string]any{"steps": []map[string]any{
		{"stepId": 2, "documentUploadId": 60},
		{"stepId": 3, "documentUploadId": 0},
		{"stepId": 1, "valueText": "  "},
		{"stepId": 4, "checked": false},
	}}
	status(t, e.do(http.MethodPut, "/organiser/verification", in), http.StatusOK)

	// Re-saving the same document keeps it (nothing purged); an empty orgName clears the name.
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{name: "Old Name", typ: "academy", status: "draft"}, true)
	e.expectActiveSteps()
	e.expectDocuments(map[int64]int64{2: 60})
	e.expectSaveProfile(nil, "academy")
	e.expectClaim(60)
	e.expectUpsert(2, nil, int64(60))
	e.mock.ExpectCommit()
	e.expectGetAfter(&prow{typ: "academy", status: "draft"}, stepText)
	status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"orgName": "", "steps": []map[string]any{{"stepId": 2, "documentUploadId": 60}}}), http.StatusOK)
}

func TestSaveValidationBeforeDB(t *testing.T) {
	e := newEnv(t)
	cases := map[string]map[string]any{
		"orgName":         {"orgName": strings.Repeat("a", 121)},
		"orgType":         {"orgType": "gym"},
		"steps[0].stepId": {"steps": []map[string]any{{"valueText": "x"}}},
		"steps[1].stepId": {"steps": []map[string]any{{"stepId": 1, "valueText": "x"}, {"stepId": 1, "valueText": "y"}}},
	}
	for field, in := range cases {
		if b := status(t, e.do(http.MethodPut, "/organiser/verification", in), http.StatusUnprocessableEntity); b.Error.Fields[field] == "" {
			t.Errorf("%s: %+v", field, b.Error)
		}
	}
	status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"orgName": "Bad\x07Name"}), http.StatusUnprocessableEntity)
	status(t, e.do(http.MethodPut, "/organiser/verification", "not an object"), http.StatusBadRequest)
}

func TestSaveStepValidation(t *testing.T) {
	cases := map[string]map[string]any{
		"steps[0].stepId":           {"stepId": 99, "valueText": "x"},
		"steps[0].valueText":        {"stepId": 1, "documentUploadId": 5},
		"steps[0].documentUploadId": {"stepId": 2, "valueText": "x"},
		"steps[0].checked":          {"stepId": 4, "valueText": "true"},
	}
	cases["long"] = map[string]any{"stepId": 1, "valueText": strings.Repeat("é", 501)}
	for field, step := range cases {
		t.Run(field, func(t *testing.T) {
			e := newEnv(t)
			e.mock.ExpectBegin()
			e.expectProfile(me, nil, true)
			e.expectActiveSteps()
			e.expectDocuments(nil)
			e.mock.ExpectRollback()
			b := status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"steps": []map[string]any{step}}), http.StatusUnprocessableEntity)
			if field == "long" {
				field = "steps[0].valueText"
			}
			if b.Error.Fields[field] == "" {
				t.Fatalf("%+v", b.Error)
			}
		})
	}
}

func TestSaveOthersOrWrongUploadIs422(t *testing.T) {
	for name, setup := range map[string]func(e *env){
		"another user's upload": func(e *env) { e.expectUpload(77, me, "", "") },
		"wrong purpose":         func(e *env) { e.expectUpload(77, me, "profile_photo", "confirmed") },
		"not uploaded":          func(e *env) { e.expectUpload(77, me, "organiser_document", "pending") },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.mock.ExpectBegin()
			e.expectProfile(me, nil, true)
			e.expectActiveSteps()
			e.expectDocuments(nil)
			e.expectSaveProfile(nil, nil)
			setup(e)
			e.mock.ExpectRollback()
			b := status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"steps": []map[string]any{{"stepId": 2, "documentUploadId": 77}}}), http.StatusUnprocessableEntity)
			if b.Error.Fields["steps[0].documentUploadId"] == "" {
				t.Fatalf("%+v", b.Error)
			}
		})
	}
}

func TestSaveAndSubmitOnlyWhenEditable(t *testing.T) {
	for _, st := range []string{"submitted", "approved"} {
		e := newEnv(t)
		e.mock.ExpectBegin()
		e.expectProfile(me, &prow{name: "X", typ: "club", status: st}, true)
		e.mock.ExpectRollback()
		if b := status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"orgName": "Y"}), http.StatusConflict); b.Error.Code != "INVALID_STATE" {
			t.Fatalf("%s: %+v", st, b.Error)
		}
		e.mock.ExpectBegin()
		e.expectProfile(me, &prow{name: "X", typ: "club", status: st}, true)
		e.mock.ExpectRollback()
		status(t, e.do(http.MethodPost, "/organiser/verification/submit", map[string]any{}), http.StatusConflict)
	}
}

func TestSaveDBErrorPurgesNothing(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{status: "draft"}, true)
	e.expectActiveSteps()
	e.expectDocuments(map[int64]int64{3: 51})
	e.expectSaveProfile(nil, nil)
	e.mock.ExpectExec(q(`DELETE FROM organiser_verification_submissions`)).WillReturnError(errors.New("db down"))
	e.mock.ExpectRollback()
	status(t, e.do(http.MethodPut, "/organiser/verification", map[string]any{"steps": []map[string]any{{"stepId": 3, "documentUploadId": 0}}}), http.StatusInternalServerError)
}

// ---- submit ----

func (e *env) expectMissing(codes ...string) {
	rows := sqlmock.NewRows([]string{"step_code"})
	for _, c := range codes {
		rows.AddRow(c)
	}
	e.mock.ExpectQuery(q(`SELECT s.step_code FROM organiser_verification_steps s LEFT JOIN organiser_verification_submissions v ON v.step_id = s.id AND v.user_id = ? WHERE s.active = 1 AND s.is_required = 1 AND v.id IS NULL`)).
		WithArgs(me).WillReturnRows(rows)
}

func TestSubmitNeedsEveryRequiredStep(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectBegin()
	e.expectProfile(me, nil, true)
	e.expectMissing("org_details", "accept_terms")
	e.mock.ExpectRollback()
	b := status(t, e.do(http.MethodPost, "/organiser/verification/submit", map[string]any{}), http.StatusUnprocessableEntity)
	f := b.Error.Fields
	if f["orgName"] == "" || f["orgType"] == "" || f["steps.org_details"] == "" || f["steps.accept_terms"] == "" || len(f) != 4 {
		t.Fatalf("fields = %v", f)
	}
}

func TestSubmitHappyPath(t *testing.T) {
	for _, from := range []string{"draft", "rejected"} {
		t.Run(from, func(t *testing.T) {
			e := newEnv(t)
			e.mock.ExpectBegin()
			e.expectProfile(me, &prow{name: "Pune Strikers", typ: "club", status: from, note: "old note"}, true)
			e.expectMissing()
			e.mock.ExpectExec(q(`UPDATE organiser_profiles SET status = 'submitted', submitted_at = ?, reviewed_by = NULL, review_note = NULL, reviewed_at = NULL WHERE user_id = ?`)).
				WithArgs(fixedNow, me).WillReturnResult(sqlmock.NewResult(0, 1))
			e.mock.ExpectExec(q(`UPDATE organiser_verification_submissions SET status = 'submitted', reviewed_by = NULL, review_note = NULL, reviewed_at = NULL WHERE user_id = ?`)).
				WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 2))
			e.mock.ExpectCommit()
			e.expectGetAfter(&prow{name: "Pune Strikers", typ: "club", status: "submitted", submittedAt: fixedNow}, filled(stepText, "x", nil), filled(stepTerms, "true", nil))
			v := decodeV(t, status(t, e.do(http.MethodPost, "/organiser/verification/submit", nil), http.StatusOK))
			if v.Status != "submitted" || !v.SubmittedAt.Equal(fixedNow) {
				t.Fatalf("v = %+v", v)
			}
		})
	}
}

// ---- admin ----

func TestAdminRoutesNeedPermission(t *testing.T) {
	e := newEnvAs(t, organiser(), "users.read")
	for _, rt := range [][2]string{{http.MethodGet, "/organisers"}, {http.MethodGet, "/organisers/7"}, {http.MethodPost, "/organisers/7/approve"}, {http.MethodPost, "/organisers/7/reject"}} {
		if b := status(t, e.doAdmin(rt[0], rt[1], map[string]any{"note": "x"}), http.StatusForbidden); b.Error.Code != "FORBIDDEN" {
			t.Fatalf("%v: %+v", rt, b.Error)
		}
	}
	if len(e.events) != 0 {
		t.Fatal("no events")
	}
}

func listRows(ids ...int64) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"user_id", "org_name", "org_type", "status", "submitted_at"})
	for _, id := range ids {
		r.AddRow(id, "Org", "club", "submitted", submitted)
	}
	return r
}

func TestAdminList(t *testing.T) {
	e := newEnv(t)
	base := `SELECT p.user_id, p.org_name, p.org_type, p.status, p.submitted_at FROM organiser_profiles p JOIN users u ON u.id = p.user_id WHERE p.status = ? AND u.status <> 'deleted'`
	e.mock.ExpectQuery(q(base+` ORDER BY p.user_id DESC LIMIT ?`)).WithArgs("submitted", 3).WillReturnRows(listRows(30, 20, 10))
	// User 20 vanished between the two reads: skipped.
	e.mock.ExpectQuery(q(`SELECT u.id, COALESCE(u.display_name,'')`)).WithArgs(int64(30), int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "display_name", "primary_role", "verification_status", "photo_upload_id", "city", "state", "sid", "scode", "sname", "pid", "pcode", "pname"}).
			AddRow(30, "Asha", "organiser", "verified", nil, nil, nil, nil, nil, nil, nil, nil, nil))
	b := status(t, e.doAdmin(http.MethodGet, "/organisers?limit=2", nil), http.StatusOK)
	var rows []AdminRow
	_ = json.Unmarshal(b.Data, &rows)
	if len(rows) != 1 || rows[0].User.ID != 30 || rows[0].User.DisplayName != "Asha" || *rows[0].OrgName != "Org" || rows[0].Status != "submitted" {
		t.Fatalf("rows = %+v", rows)
	}
	next, _ := b.Meta["nextCursor"].(string)
	if next != httpx.EncodeCursor(httpx.Cursor{ID: 20}) {
		t.Fatalf("next = %q", next)
	}
	e.mock.ExpectQuery(q(base+` AND p.user_id < ? ORDER BY p.user_id DESC LIMIT ?`)).WithArgs("approved", int64(20), 3).WillReturnRows(listRows())
	b = status(t, e.doAdmin(http.MethodGet, "/organisers?status=approved&limit=2&cursor="+next, nil), http.StatusOK)
	if string(b.Data) != "[]" || b.Meta["nextCursor"] != nil {
		t.Fatalf("page 2 = %s %v", b.Data, b.Meta)
	}
	for _, bad := range []string{"not_started", "pending"} {
		if b := status(t, e.doAdmin(http.MethodGet, "/organisers?status="+bad, nil), http.StatusUnprocessableEntity); b.Error.Fields["status"] == "" {
			t.Fatalf("%s: %+v", bad, b.Error)
		}
	}
}

func TestAdminDetail(t *testing.T) {
	e := newEnv(t)
	e.expectSummary(me)
	e.expectProfile(me, &prow{name: "Pune Strikers", typ: "club", status: "submitted", submittedAt: submitted}, false)
	e.expectSteps(me, filled(stepText, "x", nil), filled(stepProof, nil, int64(55)))
	e.expectViewURLs(55)
	e.expectAudit("organiser.view", "7", nil)
	b := status(t, e.doAdmin(http.MethodGet, "/organisers/7", nil), http.StatusOK)
	var out AdminVerification
	_ = json.Unmarshal(b.Data, &out)
	if out.User.ID != me || out.Status != "submitted" || out.Steps[1].Submission.Document.URL == nil {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(string(b.Data), `"user":{`) || !strings.Contains(string(b.Data), `"steps":[`) {
		t.Fatalf("shape: %s", b.Data)
	}

	e.expectNoSummary(8) // unknown or deleted user
	status(t, e.doAdmin(http.MethodGet, "/organisers/8", nil), http.StatusNotFound)
	e.expectSummary(9) // a user who never started
	e.expectProfile(9, nil, false)
	status(t, e.doAdmin(http.MethodGet, "/organisers/9", nil), http.StatusNotFound)
	status(t, e.doAdmin(http.MethodGet, "/organisers/x", nil), http.StatusNotFound)
}

func (e *env) expectReview(st string, note any) {
	e.mock.ExpectExec(q(`UPDATE organiser_profiles SET status = ?, reviewed_by = ?, review_note = ?, reviewed_at = ? WHERE user_id = ? AND status = 'submitted'`)).
		WithArgs(st, adminID, note, fixedNow, me).WillReturnResult(sqlmock.NewResult(0, 1))
	e.mock.ExpectExec(q(`UPDATE organiser_verification_submissions SET status = ?, reviewed_by = ?, reviewed_at = ? WHERE user_id = ?`)).
		WithArgs(st, adminID, fixedNow, me).WillReturnResult(sqlmock.NewResult(0, 3))
}

func (e *env) expectDetailAfter(st string) {
	e.expectSummary(me)
	e.expectProfile(me, &prow{name: "Org", typ: "club", status: st, reviewed: fixedNow}, false)
	e.expectSteps(me, filled(stepText, "x", nil))
}

func TestApprove(t *testing.T) {
	e := newEnv(t)
	e.expectUser(organiser())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{name: "Org", typ: "club", status: "submitted"}, true)
	e.expectReview("approved", nil)
	e.expectAudit("organiser.approve", "7", `{"withNote":false}`)
	e.mock.ExpectCommit()
	e.expectDetailAfter("approved")
	out := status(t, e.doAdmin(http.MethodPost, "/organisers/7/approve", nil), http.StatusOK)
	if v := decodeV(t, out); v.Status != "approved" {
		t.Fatalf("v = %+v", v)
	}
	if len(e.events) != 1 || e.events[0].Type != events.OrganiserApproved {
		t.Fatalf("events = %+v", e.events)
	}
	var p events.OrganiserPayload
	_ = e.events[0].Decode(&p)
	if p != (events.OrganiserPayload{UserID: me}) {
		t.Fatalf("payload = %+v", p)
	}

	// Approving again: not submitted any more → 409, no second event.
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{name: "Org", typ: "club", status: "approved"}, true)
	e.mock.ExpectRollback()
	if b := status(t, e.doAdmin(http.MethodPost, "/organisers/7/approve", map[string]any{"note": "ok"}), http.StatusConflict); b.Error.Code != "INVALID_STATE" {
		t.Fatalf("%+v", b.Error)
	}
	if len(e.events) != 1 {
		t.Fatal("one event only")
	}
}

func TestApproveWithNote(t *testing.T) {
	e := newEnv(t)
	e.expectUser(organiser())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{status: "submitted"}, true)
	e.expectReview("approved", "Welcome aboard")
	e.expectAudit("organiser.approve", "7", `{"withNote":true}`)
	e.mock.ExpectCommit()
	e.expectDetailAfter("approved")
	status(t, e.doAdmin(http.MethodPost, "/organisers/7/approve", map[string]any{"note": " Welcome aboard "}), http.StatusOK)
}

func TestReject(t *testing.T) {
	e := newEnv(t)
	for _, in := range []map[string]any{{}, {"note": "   "}, {"note": strings.Repeat("a", 501)}} {
		if b := status(t, e.doAdmin(http.MethodPost, "/organisers/7/reject", in), http.StatusUnprocessableEntity); b.Error.Fields["note"] == "" {
			t.Fatalf("%v: %+v", in, b.Error)
		}
	}
	status(t, e.doAdmin(http.MethodPost, "/organisers/7/reject", nil), http.StatusUnprocessableEntity)

	e.expectUser(organiser())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{status: "submitted"}, true)
	e.expectReview("rejected", "Proof is unreadable")
	e.expectAudit("organiser.reject", "7", `{"withNote":true}`)
	e.mock.ExpectCommit()
	e.expectDetailAfter("rejected")
	status(t, e.doAdmin(http.MethodPost, "/organisers/7/reject", map[string]any{"note": "Proof is unreadable"}), http.StatusOK)
	var p events.OrganiserPayload
	if len(e.events) != 1 || e.events[0].Type != events.OrganiserRejected || e.events[0].Decode(&p) != nil || p != (events.OrganiserPayload{UserID: me, Reason: "Proof is unreadable"}) {
		t.Fatalf("events = %+v %+v", e.events, p)
	}
}

func TestReviewNotFound(t *testing.T) {
	e := newEnv(t)
	e.mock.ExpectQuery(q(`SELECT id, display_name, primary_role, status`)).WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	status(t, e.doAdmin(http.MethodPost, "/organisers/8/approve", nil), http.StatusNotFound)

	gone := organiser()
	gone.ID, gone.Status = 9, "deleted"
	e.expectUser(gone)
	status(t, e.doAdmin(http.MethodPost, "/organisers/9/approve", nil), http.StatusNotFound)

	e.expectUser(organiser())
	e.mock.ExpectBegin()
	e.expectProfile(me, nil, true)
	e.mock.ExpectRollback()
	status(t, e.doAdmin(http.MethodPost, "/organisers/7/reject", map[string]any{"note": "x"}), http.StatusNotFound)
	if len(e.events) != 0 {
		t.Fatal("no events")
	}
}

func TestReviewAuditFailureRollsBackWithoutEvent(t *testing.T) {
	e := newEnv(t)
	e.expectUser(organiser())
	e.mock.ExpectBegin()
	e.expectProfile(me, &prow{status: "submitted"}, true)
	e.expectReview("approved", nil)
	e.mock.ExpectExec(q(`INSERT INTO admin_audit_log`)).WillReturnError(errors.New("db down"))
	e.mock.ExpectRollback()
	status(t, e.doAdmin(http.MethodPost, "/organisers/7/approve", nil), http.StatusInternalServerError)
	if len(e.events) != 0 {
		t.Fatal("no event without a commit")
	}
}

// ---- user.deleted ----

func TestUserDeletedIsIdempotent(t *testing.T) {
	e := newEnv(t)
	h := e.m.EventHandlers()[events.UserDeleted]
	ev := events.New(events.UserDeleted, me, events.UserPayload{UserID: me})
	expectWipe := func(docs ...int64) {
		rows := sqlmock.NewRows([]string{"id"})
		for _, d := range docs {
			rows.AddRow(d)
		}
		e.mock.ExpectQuery(q(`SELECT document_upload_id FROM organiser_verification_submissions WHERE user_id = ? AND document_upload_id IS NOT NULL`)).WithArgs(me).WillReturnRows(rows)
		for _, d := range docs {
			e.expectPurge(d)
		}
		e.mock.ExpectBegin()
		e.mock.ExpectExec(q(`DELETE FROM organiser_verification_submissions WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, int64(len(docs))))
		e.mock.ExpectExec(q(`UPDATE organiser_profiles SET org_name = NULL, review_note = NULL WHERE user_id = ?`)).WithArgs(me).WillReturnResult(sqlmock.NewResult(0, 1))
		e.mock.ExpectCommit()
	}
	expectWipe(50, 51)
	expectWipe() // second delivery: nothing left to purge
	for i := 0; i < 2; i++ {
		if err := h(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := h(context.Background(), events.New(events.UserDeleted, 0, events.UserPayload{})); err != nil {
		t.Fatal("malformed payload is dropped")
	}
	e.mock.ExpectQuery(q(`SELECT document_upload_id`)).WillReturnError(errors.New("db down"))
	if err := h(context.Background(), ev); err == nil {
		t.Fatal("db errors are returned for retry")
	}
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}
