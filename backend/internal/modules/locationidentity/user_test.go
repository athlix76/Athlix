package locationidentity

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

const (
	noticeRe   = `SELECT id, version, title FROM policy_documents WHERE doc_type = \? AND active = 1`
	districtRe = `SELECT d.id, d.state_id, d.name, d.active, s.code, s.name, s.is_ut\s+FROM districts d JOIN states s ON s.id = d.state_id WHERE d.id = \?`
	lockOpenRe = `SELECT id, district_id, status, verified_at FROM user_location_identities\s+WHERE user_id = \? AND identity_type = \? AND status IN \('pending','verified'\) FOR UPDATE`
)

func TestStatesAndDistricts(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	mock.ExpectQuery(`SELECT id, code, name, is_ut FROM states WHERE active = 1 ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "is_ut"}).AddRow(12, "KL", "Kerala", false).AddRow(23, "TN", "Tamil Nadu", false))
	w := testutil.Request(r, http.MethodGet, "/v1/geo/states", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("states %d %s", w.Code, w.Body)
	}
	var states []State
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &states)
	if len(states) != 2 || states[1].Code != "TN" {
		t.Fatalf("states %+v", states)
	}

	mock.ExpectQuery(`SELECT id, state_id, name FROM districts WHERE state_id = \? AND active = 1 ORDER BY name`).WithArgs(stateTN).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_id", "name"}).AddRow(coimbatore, stateTN, "Coimbatore").AddRow(thanjavur, stateTN, "Thanjavur"))
	w = testutil.Request(r, http.MethodGet, "/v1/geo/districts?stateId=23", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("districts %d %s", w.Code, w.Body)
	}
	var ds []District
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &ds)
	if len(ds) != 2 || ds[1].ID != thanjavur {
		t.Fatalf("districts %+v", ds)
	}

	w = testutil.Request(r, http.MethodGet, "/v1/geo/districts", nil, nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["stateId"] == "" {
		t.Fatalf("missing stateId %d %s", w.Code, w.Body)
	}
}

func TestMine_Slots(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	verifiedAt := now.Add(-10 * 24 * time.Hour)
	mock.ExpectQuery(`WHERE l.user_id = \? ORDER BY l.id DESC LIMIT \?`).WithArgs(me, userRowsLimit).WillReturnRows(identityRows(
		fx{ID: 12, Type: typeResidence, District: thanjavur, Status: statusVerified, VerifiedAt: verifiedAt, ExpiresAt: now.Add(40 * 24 * time.Hour)},
		fx{ID: 11, Type: typeNative, District: coimbatore, Proof: "birth_certificate", Status: statusRejected},
	))
	mock.ExpectQuery(noticeRe).WithArgs(noticeDocType).WillReturnRows(noticeRows(5))

	w := testutil.Request(r, http.MethodGet, "/v1/me/location-identities", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got LocationIdentities
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	res := got.Residence
	if res.DisplayStatus != statusVerified || res.Current == nil || res.Current.ID != 12 || res.Pending != nil || !res.CanSubmit {
		t.Fatalf("residence %+v", res)
	}
	if res.ExpiresInDays == nil || *res.ExpiresInDays != 40 {
		t.Fatalf("expiresInDays %v", res.ExpiresInDays)
	}
	if res.NextChangeAt == nil || !res.NextChangeAt.Equal(verifiedAt.Add(180*24*time.Hour)) {
		t.Fatalf("nextChangeAt %v", res.NextChangeAt)
	}
	if got.Native.DisplayStatus != statusRejected || got.Native.Current == nil || got.Native.Current.ID != 11 {
		t.Fatalf("native %+v", got.Native)
	}
	if got.Notice == nil || got.Notice.PolicyDocumentID != 5 || got.GuardianProofAllowed {
		t.Fatalf("notice/guardian %+v", got)
	}
	if res.Current.Source != sourceUpload || res.Current.State.Code != "TN" {
		t.Fatalf("current %+v", res.Current)
	}
}

func residenceInput(district int64) LocationIdentityInput {
	return LocationIdentityInput{IdentityType: typeResidence, DistrictID: district, ProofType: "rent_agreement", ProofUploadID: 99, NoticePolicyDocumentID: 5}
}

func expectSubmitHead(mock sqlmock.Sqlmock, district int64) {
	mock.ExpectQuery(noticeRe).WithArgs(noticeDocType).WillReturnRows(noticeRows(5))
	mock.ExpectQuery(districtRe).WithArgs(district).WillReturnRows(districtStateRows(district, true))
	mock.ExpectBegin()
}

// expectSubmitTail is proof reuse check, media claim, insert and commit.
func expectSubmitTail(mock sqlmock.Sqlmock, typ string, district int64, proof string) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_location_identities WHERE proof_upload_id = \?`).WithArgs(99).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(`FROM media_uploads WHERE id = \? AND user_id = \? AND status <> 'deleted'`).WithArgs(99, me).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
			AddRow(99, me, "location_proof", "location/42/99.jpg", "image/jpeg", 1000, "confirmed"))
	mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WithArgs(99, me).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO user_location_identities`).
		WithArgs(me, typ, district, stateTN, proof, "self", 99, 2, statusPending, now, nil).
		WillReturnResult(sqlmock.NewResult(70, 1))
	expectAcceptNotice(mock)
	mock.ExpectCommit()
}

// expectAcceptNotice is the policy_acceptances row for notice document 5 (idempotent insert).
func expectAcceptNotice(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`INSERT IGNORE INTO policy_acceptances \(user_id, policy_document_id\) VALUES \(\?, \?\)`).
		WithArgs(me, 5).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestSubmit_Happy(t *testing.T) {
	d, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	rec := record(d, events.LocationIdentityChanged)
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows())
	expectSubmitTail(mock, typeResidence, thanjavur, "rent_agreement")

	w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got LocationIdentity
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if got.ID != 70 || got.Status != statusPending || got.State.ID != stateTN || got.District.Name != "Thanjavur" || got.Source != sourceUpload {
		t.Fatalf("body %+v", got)
	}
	if len(rec.got) != 1 || rec.got[0].IdentityID != 70 || rec.got[0].Status != statusPending {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestSubmit_Validation(t *testing.T) {
	minor := testutil.VerifiedUser(me)
	minor.IsMinor = true
	guardian := func(typ, proof string) LocationIdentityInput {
		in := residenceInput(thanjavur)
		in.IdentityType, in.ProofType, in.ProofHolder = typ, proof, holderGuardian
		return in
	}
	cases := []struct {
		name   string
		minor  bool
		cfg    map[string]any
		in     LocationIdentityInput
		field  string
		expect func(sqlmock.Sqlmock)
	}{
		{name: "bad proof type", in: LocationIdentityInput{IdentityType: typeNative, DistrictID: thanjavur, ProofType: "rent_agreement", ProofUploadID: 99, NoticePolicyDocumentID: 5}, field: "proofType"},
		{name: "proof type removed by config", cfg: map[string]any{"location.residence_proof_types": []string{"utility_bill"}}, in: residenceInput(thanjavur), field: "proofType"},
		{name: "guardian for adult", in: guardian(typeResidence, "rent_agreement"), field: "proofHolder"},
		{name: "guardian for native", minor: true, in: guardian(typeNative, "birth_certificate"), field: "proofHolder"},
		{name: "guardian disabled", minor: true, cfg: map[string]any{"location.minor_guardian_proof_allowed": false}, in: guardian(typeResidence, "rent_agreement"), field: "proofHolder"},
		{name: "wrong notice", in: func() LocationIdentityInput { i := residenceInput(thanjavur); i.NoticePolicyDocumentID = 4; return i }(), field: "noticePolicyDocumentId",
			expect: func(m sqlmock.Sqlmock) { m.ExpectQuery(noticeRe).WithArgs(noticeDocType).WillReturnRows(noticeRows(5)) }},
		{name: "inactive district", in: residenceInput(thanjavur), field: "districtId", expect: func(m sqlmock.Sqlmock) {
			m.ExpectQuery(noticeRe).WithArgs(noticeDocType).WillReturnRows(noticeRows(5))
			m.ExpectQuery(districtRe).WithArgs(thanjavur).WillReturnRows(districtStateRows(thanjavur, false))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := testutil.VerifiedUser(me)
			if tc.minor {
				u = minor
			}
			_, mock, r := userSetup(t, tc.cfg, u)
			if tc.expect != nil {
				tc.expect(mock)
			}
			w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", tc.in, nil)
			e := decode(t, w.Body.Bytes())
			if w.Code != http.StatusUnprocessableEntity || e.Error.Fields[tc.field] == "" {
				t.Fatalf("want 422 on %s, got %d %s", tc.field, w.Code, w.Body)
			}
		})
	}
}

func TestSubmit_GuardianAllowedForMinorResidence(t *testing.T) {
	u := testutil.VerifiedUser(me)
	u.IsMinor = true
	_, mock, r := userSetup(t, nil, u)
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows())
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM user_location_identities WHERE proof_upload_id = \?`).WithArgs(99).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(`FROM media_uploads WHERE id = \?`).WithArgs(99, me).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
			AddRow(99, me, "location_proof", "k", "image/jpeg", 1000, "attached"))
	mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO user_location_identities`).
		WithArgs(me, typeResidence, thanjavur, stateTN, "rent_agreement", holderGuardian, 99, 2, statusPending, now, nil).
		WillReturnResult(sqlmock.NewResult(71, 1))
	expectAcceptNotice(mock)
	mock.ExpectCommit()
	in := residenceInput(thanjavur)
	in.ProofHolder = holderGuardian
	if w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", in, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestSubmit_ResidenceCooldown(t *testing.T) {
	verifiedAt := now.Add(-30 * 24 * time.Hour)
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows().AddRow(3, coimbatore, statusVerified, verifiedAt))
	mock.ExpectRollback()

	w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil)
	e := decode(t, w.Body.Bytes())
	want := verifiedAt.Add(180 * 24 * time.Hour).Format(time.RFC3339)
	if w.Code != http.StatusConflict || e.Error.Code != "LIMIT_REACHED" || e.Error.Fields["nextChangeAt"] != want {
		t.Fatalf("got %d %s, want nextChangeAt %s", w.Code, w.Body, want)
	}
}

func TestSubmit_CooldownFromConfig(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"location.residence_change_cooldown_days": 20}, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows().AddRow(3, coimbatore, statusVerified, now.Add(-30*24*time.Hour)))
	expectSubmitTail(mock, typeResidence, thanjavur, "rent_agreement")
	if w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestSubmit_SameDistrictRenewalAllowed(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows().AddRow(3, thanjavur, statusVerified, now.Add(-30*24*time.Hour)))
	expectSubmitTail(mock, typeResidence, thanjavur, "rent_agreement")
	if w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

func TestSubmit_PendingExists(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows().AddRow(5, coimbatore, statusPending, nil))
	mock.ExpectRollback()
	w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil)
	if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "CONFLICT" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestSubmit_DuplicatePendingKey(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeResidence).WillReturnRows(openRows())
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(`FROM media_uploads WHERE id = \?`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "purpose", "s3_key", "content_type", "size_bytes", "status"}).
			AddRow(99, me, "location_proof", "k", "image/jpeg", 1000, "attached"))
	mock.ExpectExec(`UPDATE media_uploads SET status = 'attached'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO user_location_identities`).WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	mock.ExpectRollback()
	w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", residenceInput(thanjavur), nil)
	if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "CONFLICT" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestSubmit_NativeChangeDisabled(t *testing.T) {
	_, mock, r := userSetup(t, map[string]any{"location.native_change_allowed": false}, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeNative).WillReturnRows(openRows().AddRow(2, coimbatore, statusVerified, now.Add(-400*24*time.Hour)))
	mock.ExpectRollback()
	in := LocationIdentityInput{IdentityType: typeNative, DistrictID: thanjavur, ProofType: "birth_certificate", ProofUploadID: 99, NoticePolicyDocumentID: 5}
	w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", in, nil)
	if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestSubmit_NativeChangeAllowed(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	expectSubmitHead(mock, thanjavur)
	mock.ExpectQuery(lockOpenRe).WithArgs(me, typeNative).WillReturnRows(openRows().AddRow(2, coimbatore, statusVerified, now.Add(-10*24*time.Hour)))
	expectSubmitTail(mock, typeNative, thanjavur, "birth_certificate")
	in := LocationIdentityInput{IdentityType: typeNative, DistrictID: thanjavur, ProofType: "birth_certificate", ProofUploadID: 99, NoticePolicyDocumentID: 5}
	if w := testutil.Request(r, http.MethodPost, "/v1/me/location-identities", in, nil); w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
}

const lockOwnedRe = `SELECT id, identity_type, district_id, status, proof_upload_id FROM user_location_identities\s+WHERE id = \? AND user_id = \? FOR UPDATE`

func TestWithdraw_PurgesAndPublishes(t *testing.T) {
	d, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	store := d.Storage.(*testutil.MemStorage)
	store.Put("location/42/99.jpg", "image/jpeg", 1000)
	rec := record(d, events.LocationIdentityChanged)
	mock.ExpectBegin()
	mock.ExpectQuery(lockOwnedRe).WithArgs(7, me).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "district_id", "status", "proof_upload_id"}).AddRow(7, typeResidence, thanjavur, statusPending, 99))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \? WHERE id = \? AND status = \?`).WithArgs(statusWithdrawn, 7, statusPending).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectPurge(mock, 99, "location/42/99.jpg")
	expectMarkPurged(mock, 7, 99)

	w := testutil.Request(r, http.MethodDelete, "/v1/me/location-identities/7", nil, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(store.Objects) != 0 {
		t.Fatalf("proof object not deleted")
	}
	if len(rec.got) != 1 || rec.got[0].Status != statusWithdrawn || rec.got[0].PrevStatus != statusPending {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestWithdraw_NotOwnerIs404(t *testing.T) {
	d, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	rec := record(d, events.LocationIdentityChanged)
	mock.ExpectBegin()
	mock.ExpectQuery(lockOwnedRe).WithArgs(8, me).WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "district_id", "status", "proof_upload_id"}))
	mock.ExpectRollback()
	w := testutil.Request(r, http.MethodDelete, "/v1/me/location-identities/8", nil, nil)
	if w.Code != http.StatusNotFound || len(rec.got) != 0 {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestWithdraw_RejectedIsInvalidState(t *testing.T) {
	_, mock, r := userSetup(t, nil, testutil.VerifiedUser(me))
	mock.ExpectBegin()
	mock.ExpectQuery(lockOwnedRe).WithArgs(9, me).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "district_id", "status", "proof_upload_id"}).AddRow(9, typeResidence, thanjavur, statusRejected, nil))
	mock.ExpectRollback()
	w := testutil.Request(r, http.MethodDelete, "/v1/me/location-identities/9", nil, nil)
	if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}
