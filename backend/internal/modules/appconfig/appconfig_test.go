package appconfig

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/config"
	"athlix/backend/internal/testutil"
)

type body struct {
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func expect(t *testing.T, w *httptest.ResponseRecorder, status int) body {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var b body
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b
}

var allPerms = []string{"config.read", "config.write", "policies.write", "sports.write"}

func setup(t *testing.T, cfg map[string]any, perms ...string) (*app.Deps, sqlmock.Sqlmock, *gin.Engine) {
	t.Helper()
	d, mock := testutil.Deps(t, cfg)
	fixed := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return fixed }
	r, rt := testutil.AdminEngine(7, perms)
	New(d).Routes(rt)
	return d, mock, r
}

func expectAudit(mock sqlmock.Sqlmock, action, targetType, targetID string, details any) {
	mock.ExpectExec(`INSERT INTO admin_audit_log`).WithArgs(7, action, targetType, targetID, details, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectConfigAudit(mock sqlmock.Sqlmock, table, rec string, old, new any) {
	mock.ExpectExec(`INSERT INTO config_audit`).WithArgs(table, rec, old, new, 7).WillReturnResult(sqlmock.NewResult(1, 1))
}

var (
	sportCols      = []string{"id", "code", "name", "team", "active", "sort"}
	posCols        = []string{"id", "sport_id", "code", "name", "applies", "active", "sort"}
	tplCols        = []string{"id", "sport_id", "position_id", "name", "version", "fields", "active"}
	cfgCols        = []string{"key", "value", "type", "category", "desc", "public", "version", "updated"}
	policyColNames = []string{"id", "doc_type", "version", "title", "content", "req", "active", "eff", "created"}
	reqColumns     = []string{"id", "role", "requirement", "is_required", "gates"}
	stepColumn     = []string{"id", "code", "title", "desc", "input", "req", "sort", "active"}
)

func expectCatalogue(mock sqlmock.Sqlmock, inactive bool) {
	where := `FROM sports WHERE active = 1`
	if inactive {
		where = `FROM sports ORDER BY`
	}
	mock.ExpectQuery(where).WillReturnRows(sqlmock.NewRows(sportCols).AddRow(1, "cricket", "Cricket", true, true, 0))
	mock.ExpectQuery(`FROM sport_positions`).WillReturnRows(sqlmock.NewRows(posCols).AddRow(5, 1, "batter", "Batter", "player", true, 0))
	mock.ExpectQuery(`FROM stat_templates`).WillReturnRows(sqlmock.NewRows(tplCols).AddRow(3, 1, nil, "Season", 1, `[{"key":"runs","label":"Runs","type":"int","unit":null}]`, true))
}

// ---- public ----

func TestBootstrap(t *testing.T) {
	_, mock, r := setup(t, map[string]any{"app.min_version_android": "2.1.0", "app.maintenance_mode": true})
	mock.ExpectQuery(`FROM policy_documents WHERE active = 1`).WillReturnRows(sqlmock.NewRows([]string{"id", "doc_type", "version", "title", "req"}).
		AddRow(1, "terms", 2, "Terms of Use", true).AddRow(9, "location_identity_notice", 1, "Location notice", false))
	expectCatalogue(mock, false)
	b := expect(t, testutil.Request(r, "GET", "/v1/bootstrap", nil, nil), 200)
	var out Bootstrap
	_ = json.Unmarshal(b.Data, &out)
	if len(out.Policies) != 2 || out.Policies[1].DocType != "location_identity_notice" || len(out.Sports) != 1 || len(out.Sports[0].Positions) != 1 {
		t.Fatalf("%+v", out)
	}
	if out.MinVersion.Android != "2.1.0" || out.MinVersion.IOS != "1.0.0" || !out.Maintenance || out.ServerTime.IsZero() || out.Config == nil {
		t.Fatalf("%+v", out)
	}
	// Static settings are never public, so no private key can leak through config.
	if strings.Contains(string(b.Data), "min_version_android\":") {
		t.Fatal("non-public key in config")
	}
}

func TestPublicPolicy(t *testing.T) {
	_, mock, r := setup(t, nil)
	expect(t, testutil.Request(r, "GET", "/v1/policies/nope", nil, nil), 404)

	mock.ExpectQuery(`FROM policy_documents WHERE doc_type = \? AND active = 1`).WithArgs("privacy").WillReturnRows(sqlmock.NewRows(policyColNames))
	expect(t, testutil.Request(r, "GET", "/v1/policies/privacy", nil, nil), 404)

	now := time.Now()
	mock.ExpectQuery(`FROM policy_documents WHERE doc_type = \? AND active = 1`).WithArgs("terms").
		WillReturnRows(sqlmock.NewRows(policyColNames).AddRow(1, "terms", 3, "Terms", "# T", true, true, now, now))
	b := expect(t, testutil.Request(r, "GET", "/v1/policies/terms", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"contentMd":"# T"`) || strings.Contains(string(b.Data), "createdAt") {
		t.Fatal(string(b.Data))
	}
}

// ---- permissions ----

func TestPermissions(t *testing.T) {
	_, _, r := setup(t, nil, "dashboard.read")
	cases := []struct{ method, path string }{
		{"GET", "/v1/admin/config"}, {"PUT", "/v1/admin/config/x"}, {"GET", "/v1/admin/config/x/history"},
		{"GET", "/v1/admin/policies"}, {"POST", "/v1/admin/policies"}, {"POST", "/v1/admin/policies/1/activate"},
		{"GET", "/v1/admin/verification-requirements"}, {"POST", "/v1/admin/verification-requirements"}, {"PUT", "/v1/admin/verification-requirements/1"},
		{"GET", "/v1/admin/organiser-steps"}, {"POST", "/v1/admin/organiser-steps"}, {"PUT", "/v1/admin/organiser-steps/1"},
		{"GET", "/v1/admin/sports"}, {"POST", "/v1/admin/sports"}, {"PUT", "/v1/admin/sports/1"},
		{"POST", "/v1/admin/sports/1/positions"}, {"PUT", "/v1/admin/positions/1"},
		{"POST", "/v1/admin/sports/1/stat-templates"}, {"PUT", "/v1/admin/stat-templates/1"},
	}
	for _, c := range cases {
		if w := testutil.Request(r, c.method, c.path, map[string]any{}, nil); w.Code != 403 {
			t.Errorf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
	// config.read alone cannot write.
	_, _, r = setup(t, nil, "config.read")
	expect(t, testutil.Request(r, "PUT", "/v1/admin/config/x", map[string]any{"value": 1, "version": 1}, nil), 403)
	expect(t, testutil.Request(r, "POST", "/v1/admin/policies", map[string]any{}, nil), 403)
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports", map[string]any{}, nil), 403)
}

// ---- app_config ----

func TestConfigList(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	mock.ExpectQuery(`FROM app_config WHERE category = \? ORDER BY category, config_key`).WithArgs("chat").
		WillReturnRows(sqlmock.NewRows(cfgCols).AddRow("chat.message_max_length", "2000", "int", "chat", "Max", true, 3, time.Now()))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/config?category=chat", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"value":2000`) || !strings.Contains(string(b.Data), `"valueType":"int"`) {
		t.Fatal(string(b.Data))
	}
	expect(t, testutil.Request(r, "GET", "/v1/admin/config?category="+strings.Repeat("x", 33), nil, nil), 422)
}

func cfgRow(key, value, typ string, version int) *sqlmock.Rows {
	return sqlmock.NewRows(cfgCols).AddRow(key, value, typ, "chat", "d", false, version, time.Now())
}

func TestPutConfig(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \? FOR UPDATE`).WithArgs("chat.message_max_length").WillReturnRows(cfgRow("chat.message_max_length", "2000", "int", 3))
	mock.ExpectExec(`UPDATE app_config SET value = \?, version = \?, updated_by = \?, updated_at = \? WHERE config_key = \?`).
		WithArgs("1500", 4, 7, sqlmock.AnyArg(), "chat.message_max_length").WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "app_config", "chat.message_max_length", "2000", "1500")
	expectAudit(mock, "config.update", "app_config", "chat.message_max_length", `{"version":4}`)
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "PUT", "/v1/admin/config/chat.message_max_length", map[string]any{"value": 1500, "version": 3}, nil), 200)
	var e ConfigEntry
	_ = json.Unmarshal(b.Data, &e)
	if e.Version != 4 || string(e.Value) != "1500" {
		t.Fatalf("%+v", e)
	}
}

func TestPutConfigErrors(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	// missing version
	expect(t, testutil.Request(r, "PUT", "/v1/admin/config/k", map[string]any{"value": 1}, nil), 422)

	// unknown key
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \?`).WithArgs("nope").WillReturnRows(sqlmock.NewRows(cfgCols))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/config/nope", map[string]any{"value": 1, "version": 1}, nil), 404)

	// wrong type
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \?`).WillReturnRows(cfgRow("k", "1", "int", 1))
	mock.ExpectRollback()
	b := expect(t, testutil.Request(r, "PUT", "/v1/admin/config/k", map[string]any{"value": "ten", "version": 1}, nil), 422)
	if b.Error.Fields["value"] == "" {
		t.Fatal(b)
	}

	// stale version
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \?`).WillReturnRows(cfgRow("k", "1", "int", 5))
	mock.ExpectRollback()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/config/k", map[string]any{"value": 2, "version": 4}, nil), 409)
	if b.Error.Code != "CONFLICT" {
		t.Fatal(b.Error.Code)
	}

	// enum-like key outside its allowed values
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \?`).WillReturnRows(cfgRow("minor.dm_policy", `"none"`, "string", 1))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/config/minor.dm_policy", map[string]any{"value": "everyone", "version": 1}, nil), 422)

	// unchanged value: no write, no audit
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM app_config WHERE config_key = \?`).WillReturnRows(cfgRow("k", "7", "int", 2))
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/config/k", map[string]any{"value": 7, "version": 2}, nil), 200)
	if !strings.Contains(string(b.Data), `"version":2`) {
		t.Fatal(string(b.Data))
	}
}

func TestValidateValue(t *testing.T) {
	d, _ := testutil.Deps(t, nil)
	s := newService(d)
	cases := []struct {
		key, typ, raw string
		ok            bool
	}{
		{"k", "int", `10`, true}, {"k", "int", `1.5`, false}, {"k", "int", `-1`, false}, {"k", "int", `"1"`, false},
		{"k", "decimal", `1.5`, true}, {"k", "decimal", `true`, false},
		{"k", "bool", `false`, true}, {"k", "bool", `0`, false},
		{"k", "string", `"x"`, true}, {"k", "string", `1`, false}, {"k", "string", `"` + strings.Repeat("a", 1001) + `"`, false},
		{"k", "string_list", `["a","b"]`, true}, {"k", "string_list", `["a",1]`, false}, {"k", "string_list", `[""]`, false}, {"k", "string_list", `"a"`, false},
		{"k", "json", `{"a":1}`, true}, {"k", "json", `[1]`, false},
		{"k", "int", ``, false}, {"k", "int", `null`, false}, {"k", "int", `1 2`, false}, {"k", "weird", `1`, false},
		{"auth.sms_provider", "string", `"msg91"`, true}, {"auth.sms_provider", "string", `"twilio"`, false},
		{"verification.provider", "string", `"mock"`, true}, {"verification.provider", "string", `"aggregator"`, true},
		{"minor.profile_default_visibility", "string", `"followers"`, true}, {"minor.profile_default_visibility", "string", `"friends"`, false},
		{"event.show_contact", "string", `"always"`, true}, {"event.show_contact", "string", `"never"`, false},
		{"event.eligibility_default_government", "string", `"native"`, true}, {"event.eligibility_default_private", "string", `"local"`, false},
		{"listing.payment_provider", "string", `"razorpay"`, true}, {"listing.payment_provider", "string", `"stripe"`, false},
		{"app.min_version_ios", "string", `"1.2.3"`, true}, {"app.min_version_ios", "string", `"1.2"`, false},
		{"app.support_email", "string", `"help@athlix.test"`, true}, {"app.support_email", "string", `"not an email"`, false},
	}
	for _, c := range cases {
		_, msg := s.validateValue(c.key, c.typ, json.RawMessage(c.raw))
		if (msg == "") != c.ok {
			t.Errorf("%s %s %s: msg=%q", c.key, c.typ, c.raw, msg)
		}
	}
	out, msg := s.validateValue("k", "json", json.RawMessage(`{ "a" : 1 }`))
	if msg != "" || string(out) != `{"a":1}` {
		t.Fatal(string(out), msg)
	}

	d.Cfg.Env = config.EnvProd
	for _, k := range []string{"verification.provider", "listing.payment_provider"} {
		if _, msg := s.validateValue(k, "string", json.RawMessage(`"mock"`)); msg == "" {
			t.Errorf("%s: mock accepted in prod", k)
		}
	}
}

func TestConfigHistory(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM app_config WHERE config_key = \?`).WithArgs("nope").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	expect(t, testutil.Request(r, "GET", "/v1/admin/config/nope/history", nil, nil), 404)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM app_config`).WithArgs("k").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery(`FROM config_audit c LEFT JOIN admin_users a ON a.id = c.changed_by WHERE c.table_name = 'app_config' AND c.record_id = \? ORDER BY c.id DESC LIMIT \?`).
		WithArgs("k", 100).WillReturnRows(sqlmock.NewRows([]string{"o", "n", "by", "name", "at"}).AddRow("1", "2", 7, "Ops", time.Now()).AddRow(nil, "1", 3, "", time.Now()))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/config/k/history", nil, nil), 200)
	var out []ConfigChange
	_ = json.Unmarshal(b.Data, &out)
	if len(out) != 2 || string(out[0].NewValue) != "2" || out[0].ChangedBy.Name != "Ops" || string(out[1].OldValue) != "null" {
		t.Fatalf("%s", b.Data)
	}
}

// ---- policies ----

func TestAdminPolicies(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	expect(t, testutil.Request(r, "GET", "/v1/admin/policies?docType=bogus", nil, nil), 422)
	now := time.Now()
	mock.ExpectQuery(`FROM policy_documents WHERE doc_type = \? ORDER BY doc_type, version DESC`).WithArgs("terms").
		WillReturnRows(sqlmock.NewRows(policyColNames).AddRow(2, "terms", 2, "T2", "#", true, false, now, now).AddRow(1, "terms", 1, "T1", "#", true, true, now, now))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/policies?docType=terms", nil, nil), 200)
	var out []AdminPolicy
	_ = json.Unmarshal(b.Data, &out)
	if len(out) != 2 || out[0].Active || !out[1].Active || out[0].CreatedAt.IsZero() {
		t.Fatalf("%s", b.Data)
	}
}

func TestCreatePolicy(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/policies", map[string]any{"docType": "faq", "title": "", "contentMd": " "}, nil), 422)
	if len(b.Error.Fields) != 3 {
		t.Fatal(b.Error.Fields)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(version\), 0\) FROM policy_documents WHERE doc_type = \? FOR UPDATE`).WithArgs("location_identity_notice").
		WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO policy_documents`).WithArgs("location_identity_notice", 2, "Notice", "# Body", true, sqlmock.AnyArg(), 7, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(14, 1))
	expectConfigAudit(mock, "policy_documents", "14", nil, `{"active":false,"docType":"location_identity_notice","requiresAcceptance":true,"title":"Notice","version":2}`)
	expectAudit(mock, "policy.create", "policy_document", "14", `{"version":2}`)
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", "/v1/admin/policies",
		map[string]any{"docType": "location_identity_notice", "title": " Notice ", "contentMd": "# Body", "requiresAcceptance": true}, nil), 201)
	var p AdminPolicy
	_ = json.Unmarshal(b.Data, &p)
	if p.ID != 14 || p.Version != 2 || p.Active || p.Title != "Notice" {
		t.Fatalf("%+v", p)
	}
}

func TestActivatePolicy(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM policy_documents WHERE id = \? FOR UPDATE`).WithArgs(2).
		WillReturnRows(sqlmock.NewRows(policyColNames).AddRow(2, "terms", 2, "T2", "#", true, false, now, now))
	mock.ExpectQuery(`SELECT id FROM policy_documents WHERE doc_type = \? AND active = 1 FOR UPDATE`).WithArgs("terms").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`UPDATE policy_documents SET active = 0 WHERE doc_type = \? AND active = 1`).WithArgs("terms").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE policy_documents SET active = 1, effective_from = \? WHERE id = \?`).WithArgs(sqlmock.AnyArg(), 2).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "policy_documents", "1", `{"active":true}`, `{"active":false}`)
	expectConfigAudit(mock, "policy_documents", "2", `{"active":false}`, `{"active":true}`)
	expectAudit(mock, "policy.activate", "policy_document", "2", `{"deactivatedIds":[1]}`)
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/policies/2/activate", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"effectiveFrom":"2026-11-13T10:00:00Z"`) {
		t.Fatal(string(b.Data))
	}

	// already active: no writes
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM policy_documents WHERE id = \?`).WithArgs(1).WillReturnRows(sqlmock.NewRows(policyColNames).AddRow(1, "terms", 1, "T", "#", true, true, now, now))
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "POST", "/v1/admin/policies/1/activate", nil, nil), 200)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM policy_documents WHERE id = \?`).WithArgs(99).WillReturnRows(sqlmock.NewRows(policyColNames))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/policies/99/activate", nil, nil), 404)
}

// ---- verification requirements ----

func TestRequirements(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	mock.ExpectQuery(`FROM verification_requirements ORDER BY`).WillReturnRows(sqlmock.NewRows(reqColumns).AddRow(1, "player", "digilocker", true, "app_access"))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/verification-requirements", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"gates":"app_access"`) {
		t.Fatal(string(b.Data))
	}

	b = expect(t, testutil.Request(r, "POST", "/v1/admin/verification-requirements", map[string]any{"role": "admin", "requirement": "x", "gates": "y"}, nil), 422)
	if len(b.Error.Fields) != 4 {
		t.Fatal(b.Error.Fields)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO verification_requirements`).WithArgs("coach", "digilocker", false, "create_community").WillReturnResult(sqlmock.NewResult(8, 1))
	expectConfigAudit(mock, "verification_requirements", "8", nil, `{"id":8,"role":"coach","requirement":"digilocker","isRequired":false,"gates":"create_community"}`)
	expectAudit(mock, "verification_requirement.create", "verification_requirement", "8", `{"isRequired":false}`)
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "POST", "/v1/admin/verification-requirements",
		map[string]any{"role": "coach", "requirement": "digilocker", "isRequired": false, "gates": "create_community"}, nil), 201)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO verification_requirements`).WillReturnError(dupErr)
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/verification-requirements",
		map[string]any{"role": "coach", "requirement": "digilocker", "isRequired": true, "gates": "app_access"}, nil), 409)

	expect(t, testutil.Request(r, "PUT", "/v1/admin/verification-requirements/1", map[string]any{}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM verification_requirements WHERE id = \? FOR UPDATE`).WithArgs(1).WillReturnRows(sqlmock.NewRows(reqColumns).AddRow(1, "player", "digilocker", true, "app_access"))
	mock.ExpectExec(`UPDATE verification_requirements SET is_required = \? WHERE id = \?`).WithArgs(false, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "verification_requirements", "1", sqlmock.AnyArg(), `{"id":1,"role":"player","requirement":"digilocker","isRequired":false,"gates":"app_access"}`)
	expectAudit(mock, "verification_requirement.update", "verification_requirement", "1", `{"isRequired":false}`)
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/verification-requirements/1", map[string]any{"isRequired": false}, nil), 200)
	if !strings.Contains(string(b.Data), `"isRequired":false`) {
		t.Fatal(string(b.Data))
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM verification_requirements WHERE id = \?`).WithArgs(5).WillReturnRows(sqlmock.NewRows(reqColumns))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/verification-requirements/5", map[string]any{"isRequired": true}, nil), 404)
}

type dupError struct{}

func (dupError) Error() string { return "Error 1062 (23000): Duplicate entry 'x' for key 'uq'" }

var dupErr = dupError{}

// ---- organiser steps ----

func TestOrganiserSteps(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	mock.ExpectQuery(`FROM organiser_verification_steps ORDER BY sort_order, id`).WillReturnRows(sqlmock.NewRows(stepColumn).AddRow(1, "gst", "GST", nil, "text", true, 0, true))
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/organiser-steps", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"stepCode":"gst"`) || !strings.Contains(string(b.Data), `"description":null`) {
		t.Fatal(string(b.Data))
	}

	b = expect(t, testutil.Request(r, "POST", "/v1/admin/organiser-steps", map[string]any{"stepCode": "Bad Code", "inputType": "video"}, nil), 422)
	if len(b.Error.Fields) != 3 {
		t.Fatal(b.Error.Fields)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO organiser_verification_steps`).WithArgs("pan_card", "PAN card", nil, "document", true, 2, true).WillReturnResult(sqlmock.NewResult(4, 1))
	expectConfigAudit(mock, "organiser_verification_steps", "4", nil, sqlmock.AnyArg())
	expectAudit(mock, "organiser_step.create", "organiser_step", "4", nil)
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "POST", "/v1/admin/organiser-steps", map[string]any{"stepCode": "pan_card", "title": "PAN card", "description": "  ", "inputType": "document", "sortOrder": 2}, nil), 201)
	var st OrganiserStep
	_ = json.Unmarshal(b.Data, &st)
	if st.ID != 4 || !st.IsRequired || !st.Active || st.Description != nil {
		t.Fatalf("%+v", st)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM organiser_verification_steps WHERE id = \? FOR UPDATE`).WithArgs(4).WillReturnRows(sqlmock.NewRows(stepColumn).AddRow(4, "pan_card", "PAN card", nil, "document", true, 2, true))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/organiser-steps/4", map[string]any{"stepCode": "other"}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM organiser_verification_steps WHERE id = \?`).WithArgs(4).WillReturnRows(sqlmock.NewRows(stepColumn).AddRow(4, "pan_card", "PAN card", nil, "document", true, 2, true))
	mock.ExpectExec(`UPDATE organiser_verification_steps SET`).WithArgs("PAN card", "Upload a photo", "document", true, 2, false, 4).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "organiser_verification_steps", "4", sqlmock.AnyArg(), sqlmock.AnyArg())
	expectAudit(mock, "organiser_step.update", "organiser_step", "4", `{"active":false,"isRequired":true}`)
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/organiser-steps/4", map[string]any{"description": "Upload a photo", "active": false}, nil), 200)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM organiser_verification_steps WHERE id = \?`).WithArgs(9).WillReturnRows(sqlmock.NewRows(stepColumn))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/organiser-steps/9", map[string]any{"active": false}, nil), 404)
}

// ---- sports ----

func TestAdminSportsCatalogue(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	expectCatalogue(mock, true)
	b := expect(t, testutil.Request(r, "GET", "/v1/admin/sports", nil, nil), 200)
	if !strings.Contains(string(b.Data), `"sortOrder":0`) || !strings.Contains(string(b.Data), `"active":true`) {
		t.Fatal(string(b.Data))
	}
}

func TestCreateAndUpdateSport(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports", map[string]any{"code": "Cricket!", "name": ""}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sports`).WithArgs("kabaddi", "Kabaddi", true, 3).WillReturnResult(sqlmock.NewResult(6, 1))
	expectConfigAudit(mock, "sports", "6", nil, `{"id":6,"code":"kabaddi","name":"Kabaddi","isTeamSport":true,"sortOrder":3,"active":true}`)
	expectAudit(mock, "sport.create", "sport", "6", nil)
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/sports", map[string]any{"code": "kabaddi", "name": "Kabaddi", "isTeamSport": true, "sortOrder": 3}, nil), 201)
	if !strings.Contains(string(b.Data), `"positions":[]`) || !strings.Contains(string(b.Data), `"sortOrder":3`) {
		t.Fatal(string(b.Data))
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sports`).WillReturnError(dupErr)
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports", map[string]any{"code": "kabaddi", "name": "Kabaddi"}, nil), 409)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \? FOR UPDATE`).WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "team", "sort", "active"}).AddRow(1, "cricket", "Cricket", true, 0, true))
	mock.ExpectExec(`UPDATE sports SET name = \?, is_team_sport = \?, sort_order = \?, active = \? WHERE id = \?`).WithArgs("Cricket", true, 0, false, 1).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "sports", "1", sqlmock.AnyArg(), sqlmock.AnyArg())
	expectAudit(mock, "sport.update", "sport", "1", `{"active":false}`)
	mock.ExpectCommit()
	expectCatalogue(mock, true)
	expect(t, testutil.Request(r, "PUT", "/v1/admin/sports/1", map[string]any{"active": false}, nil), 200)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \?`).WithArgs(77).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "team", "sort", "active"}))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/sports/77", map[string]any{"active": false}, nil), 404)
}

func TestPositions(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/sports/1/positions", map[string]any{"code": "x", "name": "", "appliesTo": "fan"}, nil), 422)
	if len(b.Error.Fields) != 3 {
		t.Fatal(b.Error.Fields)
	}

	sportRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "code", "name", "team", "sort", "active"}).AddRow(1, "cricket", "Cricket", true, 0, true)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \?`).WithArgs(2).WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "team", "sort", "active"}))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports/2/positions", map[string]any{"code": "keeper", "name": "Keeper", "appliesTo": "player"}, nil), 404)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \?`).WithArgs(1).WillReturnRows(sportRows())
	mock.ExpectExec(`INSERT INTO sport_positions`).WithArgs(1, "keeper", "Keeper", "both", 1).WillReturnResult(sqlmock.NewResult(11, 1))
	expectConfigAudit(mock, "sport_positions", "11", nil, sqlmock.AnyArg())
	expectAudit(mock, "sport_position.create", "sport_position", "11", `{"sportId":1}`)
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports/1/positions", map[string]any{"code": "keeper", "name": "Keeper", "appliesTo": "both", "sortOrder": 1}, nil), 201)

	posRow := sqlmock.NewRows([]string{"id", "sport_id", "code", "name", "applies", "sort", "active"}).AddRow(11, 1, "keeper", "Keeper", "both", 1, true)
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sport_positions WHERE id = \? FOR UPDATE`).WithArgs(11).WillReturnRows(posRow)
	mock.ExpectExec(`UPDATE sport_positions SET`).WithArgs("Keeper", "coach", 1, true, 11).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "sport_positions", "11", sqlmock.AnyArg(), sqlmock.AnyArg())
	expectAudit(mock, "sport_position.update", "sport_position", "11", `{"active":true}`)
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/positions/11", map[string]any{"appliesTo": "coach"}, nil), 200)
	if !strings.Contains(string(b.Data), `"appliesTo":"coach"`) {
		t.Fatal(string(b.Data))
	}
	expect(t, testutil.Request(r, "PUT", "/v1/admin/positions/11", map[string]any{"appliesTo": "fan"}, nil), 422)
}

func TestStatTemplates(t *testing.T) {
	_, mock, r := setup(t, nil, allPerms...)
	sportRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "code", "name", "team", "sort", "active"}).AddRow(1, "cricket", "Cricket", true, 0, true)
	}
	bad := []map[string]any{
		{"name": "S", "fields": []any{}},
		{"name": "S", "fields": []any{map[string]any{"key": "runs", "label": "Runs", "type": "int"}, map[string]any{"key": "runs", "label": "R", "type": "int"}}},
		{"name": "S", "fields": []any{map[string]any{"key": "Runs", "label": "Runs", "type": "int"}}},
		{"name": "S", "fields": []any{map[string]any{"key": "runs", "label": "Runs", "type": "money"}}},
		{"name": "S", "fields": []any{map[string]any{"key": "runs", "label": "", "type": "int"}}},
		{"name": "", "fields": []any{map[string]any{"key": "runs", "label": "Runs", "type": "int"}}},
	}
	for i, in := range bad {
		if w := testutil.Request(r, "POST", "/v1/admin/sports/1/stat-templates", in, nil); w.Code != 422 {
			t.Errorf("case %d: %d", i, w.Code)
		}
	}

	fields := []any{map[string]any{"key": "runs", "label": "Runs", "type": "int", "unit": " "}}
	// position of another sport
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \?`).WithArgs(1).WillReturnRows(sportRows())
	mock.ExpectQuery(`SELECT sport_id FROM sport_positions WHERE id = \?`).WithArgs(40).WillReturnRows(sqlmock.NewRows([]string{"sport_id"}).AddRow(2))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "POST", "/v1/admin/sports/1/stat-templates", map[string]any{"positionId": 40, "name": "Batting", "fields": fields}, nil), 422)

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM sports WHERE id = \?`).WithArgs(1).WillReturnRows(sportRows())
	mock.ExpectQuery(`SELECT sport_id FROM sport_positions`).WithArgs(5).WillReturnRows(sqlmock.NewRows([]string{"sport_id"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO stat_templates`).WithArgs(1, 5, "Batting", `[{"key":"runs","label":"Runs","type":"int","unit":null}]`).WillReturnResult(sqlmock.NewResult(20, 1))
	expectConfigAudit(mock, "stat_templates", "20", nil, sqlmock.AnyArg())
	expectAudit(mock, "stat_template.create", "stat_template", "20", `{"sportId":1}`)
	mock.ExpectCommit()
	b := expect(t, testutil.Request(r, "POST", "/v1/admin/sports/1/stat-templates", map[string]any{"positionId": 5, "name": "Batting", "fields": fields}, nil), 201)
	if !strings.Contains(string(b.Data), `"version":1`) || !strings.Contains(string(b.Data), `"positionId":5`) {
		t.Fatal(string(b.Data))
	}

	tplRow := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "sport_id", "position_id", "name", "fields", "version", "active"}).
			AddRow(20, 1, 5, "Batting", `[{"key":"runs","label":"Runs","type":"int","unit":null}]`, 1, true)
	}
	// rename only: version stays
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM stat_templates WHERE id = \? FOR UPDATE`).WithArgs(20).WillReturnRows(tplRow())
	mock.ExpectExec(`UPDATE stat_templates SET`).WithArgs("Bat", `[{"key":"runs","label":"Runs","type":"int","unit":null}]`, 1, true, 20).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "stat_templates", "20", sqlmock.AnyArg(), sqlmock.AnyArg())
	expectAudit(mock, "stat_template.update", "stat_template", "20", `{"active":true,"version":1}`)
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/stat-templates/20", map[string]any{"name": "Bat"}, nil), 200)

	// same fields: no write
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM stat_templates WHERE id = \?`).WithArgs(20).WillReturnRows(tplRow())
	mock.ExpectCommit()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/stat-templates/20", map[string]any{"fields": fields}, nil), 200)

	// changed fields: version bumps
	mock.ExpectBegin()
	mock.ExpectQuery(`FROM stat_templates WHERE id = \?`).WithArgs(20).WillReturnRows(tplRow())
	mock.ExpectExec(`UPDATE stat_templates SET`).WithArgs("Batting", sqlmock.AnyArg(), 2, true, 20).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConfigAudit(mock, "stat_templates", "20", sqlmock.AnyArg(), sqlmock.AnyArg())
	expectAudit(mock, "stat_template.update", "stat_template", "20", `{"active":true,"version":2}`)
	mock.ExpectCommit()
	b = expect(t, testutil.Request(r, "PUT", "/v1/admin/stat-templates/20", map[string]any{"fields": append(fields, map[string]any{"key": "sr", "label": "Strike rate", "type": "percent"})}, nil), 200)
	if !strings.Contains(string(b.Data), `"version":2`) {
		t.Fatal(string(b.Data))
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`FROM stat_templates WHERE id = \?`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"id", "sport_id", "position_id", "name", "fields", "version", "active"}))
	mock.ExpectRollback()
	expect(t, testutil.Request(r, "PUT", "/v1/admin/stat-templates/99", map[string]any{"active": false}, nil), 404)
}
