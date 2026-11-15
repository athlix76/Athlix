package locationidentity

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/testutil"
)

const (
	lockReviewRe   = `SELECT id, user_id, identity_type, district_id, proof_type, proof_upload_id, status\s+FROM user_location_identities WHERE id = \? FOR UPDATE`
	lockVerifiedRe = `SELECT id, district_id FROM user_location_identities\s+WHERE user_id = \? AND identity_type = \? AND status = 'verified' FOR UPDATE`
	adminAuditRe   = `INSERT INTO admin_audit_log`
)

func TestAdmin_ForbiddenWithoutPermission(t *testing.T) {
	_, _, r := adminSetup(t, nil, "config.read")
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/v1/admin/location-identities"},
		{http.MethodGet, "/v1/admin/location-identities/7"},
		{http.MethodPost, "/v1/admin/location-identities/7/approve"},
		{http.MethodPost, "/v1/admin/location-identities/7/reject"},
		{http.MethodPost, "/v1/admin/location-identities/7/revoke"},
		{http.MethodPost, "/v1/admin/geo/districts"},
		{http.MethodPut, "/v1/admin/geo/districts/26"},
	} {
		if w := testutil.Request(r, c.method, c.path, map[string]any{}, nil); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
}

func TestAdmin_QueuePendingOldestFirst(t *testing.T) {
	_, mock, r := adminSetup(t, nil, permReview)
	cols := append(append([]string{}, identityColNames...), "is_minor")
	rows := sqlmock.NewRows(cols).
		AddRow(append(fx{ID: 5, Type: typeResidence, District: thanjavur, Status: statusPending, Upload: 99}.values(), true)...)
	mock.ExpectQuery(`JOIN users u ON u.id = l.user_id WHERE l.status = \? AND l.identity_type = \? ORDER BY l.id ASC LIMIT \?`).
		WithArgs(statusPending, typeResidence, adminPageSize+1).WillReturnRows(rows)
	mock.ExpectQuery(summariesRe).WithArgs(me).WillReturnRows(summaryRows(me))

	w := testutil.Request(r, http.MethodGet, "/v1/admin/location-identities?identityType=residence", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got []AdminLocationRow
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if len(got) != 1 || got[0].ID != 5 || !got[0].IsMinor || got[0].User.ID != me {
		t.Fatalf("rows %s", w.Body)
	}

	w = testutil.Request(r, http.MethodGet, "/v1/admin/location-identities?status=nope", nil, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status filter %d", w.Code)
	}
}

func TestAdmin_DetailSignsProofAndAudits(t *testing.T) {
	_, mock, r := adminSetup(t, nil, permReview)
	expectDetail(mock, fx{ID: 7, Type: typeResidence, District: thanjavur, Status: statusPending, Upload: 99})
	mock.ExpectQuery(`SELECT s3_key, purpose FROM media_uploads WHERE id = \? AND status IN`).WithArgs(99).
		WillReturnRows(sqlmock.NewRows([]string{"s3_key", "purpose"}).AddRow("location/42/99.jpg", "location_proof"))
	mock.ExpectExec(adminAuditRe).WithArgs(adminID, "location.proof_viewed", "location_identity", "7", `{"uploadId":99}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	w := testutil.Request(r, http.MethodGet, "/v1/admin/location-identities/7", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got map[string]any
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if got["proofUrl"] != "https://storage.test/get/location/42/99.jpg" || got["proofContentType"] != "image/jpeg" {
		t.Fatalf("detail %s", w.Body)
	}
	for _, k := range []string{"aadhaarLast4", "reviewedBy", "expiresAt", "rejectionCode", "proofPurgedAt"} {
		if v, ok := got[k]; !ok || v != nil {
			t.Fatalf("%s must be present and null: %s", k, w.Body)
		}
	}
}

func TestAdmin_ApproveSupersedesAndSetsExpiryFromConfig(t *testing.T) {
	d, mock, r := adminSetup(t, map[string]any{"location.residence_validity_months": 6}, permReview)
	rec := record(d, events.LocationIdentityChanged)
	expires := now.AddDate(0, 6, 0)
	mock.ExpectBegin()
	mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeResidence, "rent_agreement", statusPending, 99))
	mock.ExpectQuery(lockVerifiedRe).WithArgs(me, typeResidence).WillReturnRows(sqlmock.NewRows([]string{"id", "district_id"}).AddRow(3, coimbatore))
	mock.ExpectExec(`UPDATE user_location_identities SET status = \? WHERE id = \? AND status = \?`).WithArgs(statusSuperseded, 3, statusVerified).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_location_identities SET status = 'verified'`).WithArgs(now, adminID, now, expires, nil, "ok", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(adminAuditRe).WithArgs(adminID, "location.approve", "location_identity", "7", `{"identityType":"residence","supersededId":3,"withNote":true}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	expectDetail(mock, fx{ID: 7, Type: typeResidence, District: thanjavur, Status: statusVerified, Upload: 99, VerifiedAt: now, ExpiresAt: expires, Note: "ok"})

	w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/approve", map[string]any{"note": " ok "}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var det AdminLocationDetail
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &det)
	if det.ReviewNote == nil || *det.ReviewNote != "ok" {
		t.Fatalf("reviewNote %v", det.ReviewNote)
	}
	if len(rec.got) != 2 || rec.got[0].IdentityID != 3 || rec.got[0].Status != statusSuperseded ||
		rec.got[1].IdentityID != 7 || rec.got[1].Status != statusVerified || rec.got[1].ExpiresAt != "2027-05-13T10:00:00Z" {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestAdmin_ApproveNoteTooLong(t *testing.T) {
	_, mock, r := adminSetup(t, nil, permReview)
	w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/approve", map[string]any{"note": strings.Repeat("a", 256)}, nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["note"] == "" {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdmin_ApproveLast4(t *testing.T) {
	t.Run("valid on masked aadhaar, native has no expiry", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, permReview)
		mock.ExpectBegin()
		mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeNative, proofMaskedAadhaar, statusPending, 99))
		mock.ExpectQuery(lockVerifiedRe).WithArgs(me, typeNative).WillReturnRows(sqlmock.NewRows([]string{"id", "district_id"}))
		mock.ExpectExec(`UPDATE user_location_identities SET status = 'verified'`).WithArgs(now, adminID, now, nil, "1234", nil, 7).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(adminAuditRe).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		expectDetail(mock, fx{ID: 7, Type: typeNative, District: thanjavur, Proof: proofMaskedAadhaar, Status: statusVerified, VerifiedAt: now})
		if w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/approve", map[string]any{"aadhaarLast4": "1234"}, nil); w.Code != http.StatusOK {
			t.Fatalf("status %d %s", w.Code, w.Body)
		}
	})
	for _, c := range []struct{ name, proof, last4 string }{
		{"not 4 digits", proofMaskedAadhaar, "12a4"},
		{"too long", proofMaskedAadhaar, "12345"},
		{"wrong proof type", "birth_certificate", "1234"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, mock, r := adminSetup(t, nil, permReview)
			mock.ExpectBegin()
			mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeNative, c.proof, statusPending, 99))
			mock.ExpectRollback()
			w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/approve", map[string]any{"aadhaarLast4": c.last4}, nil)
			if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["aadhaarLast4"] == "" {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestAdmin_RejectUnmaskedAadhaarPurgesProof(t *testing.T) {
	d, mock, r := adminSetup(t, nil, permReview)
	store := d.Storage.(*testutil.MemStorage)
	store.Put("location/42/99.jpg", "image/jpeg", 1000)
	rec := record(d, events.LocationIdentityChanged)
	mock.ExpectBegin()
	mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeNative, proofMaskedAadhaar, statusPending, 99))
	mock.ExpectExec(`UPDATE user_location_identities SET status = 'rejected'`).WithArgs(rejectUnmaskedAadhaar, nil, now, adminID, 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(adminAuditRe).WithArgs(adminID, "location.reject", "location_identity", "7", `{"rejectionCode":"unmasked_aadhaar"}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	expectPurge(mock, 99, "location/42/99.jpg")
	expectMarkPurged(mock, 7, 99)
	expectDetail(mock, fx{ID: 7, Type: typeNative, District: thanjavur, Proof: proofMaskedAadhaar, Status: statusRejected})

	w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/reject", map[string]any{"rejectionCode": rejectUnmaskedAadhaar}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	if len(store.Objects) != 0 {
		t.Fatal("proof object not deleted")
	}
	if len(rec.got) != 1 || rec.got[0].Status != statusRejected || rec.got[0].RejectionCode != rejectUnmaskedAadhaar {
		t.Fatalf("events %+v", rec.got)
	}
}

func TestAdmin_RejectValidatesCode(t *testing.T) {
	_, _, r := adminSetup(t, map[string]any{"location.rejection_reasons": []string{"other"}}, permReview)
	w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/reject", map[string]any{"rejectionCode": "unclear_document"}, nil)
	if w.Code != http.StatusUnprocessableEntity || decode(t, w.Body.Bytes()).Error.Fields["rejectionCode"] == "" {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestAdmin_RevokeOnlyVerified(t *testing.T) {
	t.Run("pending is invalid state", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, permReview)
		mock.ExpectBegin()
		mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeResidence, "rent_agreement", statusPending, 99))
		mock.ExpectRollback()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/revoke", map[string]any{"reason": "fraud"}, nil)
		if w.Code != http.StatusConflict || decode(t, w.Body.Bytes()).Error.Code != "INVALID_STATE" {
			t.Fatalf("got %d %s", w.Code, w.Body)
		}
	})
	t.Run("verified is revoked", func(t *testing.T) {
		d, mock, r := adminSetup(t, nil, permReview)
		rec := record(d, events.LocationIdentityChanged)
		mock.ExpectBegin()
		mock.ExpectQuery(lockReviewRe).WithArgs(7).WillReturnRows(reviewRows(7, typeResidence, "rent_agreement", statusVerified, nil))
		mock.ExpectExec(`UPDATE user_location_identities SET status = 'revoked', rejection_reason = \? WHERE id = \? AND status = 'verified'`).
			WithArgs("fraud", 7).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(adminAuditRe).WithArgs(adminID, "location.revoke", "location_identity", "7", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		expectDetail(mock, fx{ID: 7, Type: typeResidence, District: thanjavur, Status: statusRevoked})
		w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/revoke", map[string]any{"reason": "fraud"}, nil)
		if w.Code != http.StatusOK || len(rec.got) != 1 || rec.got[0].Status != statusRevoked {
			t.Fatalf("got %d %s %+v", w.Code, w.Body, rec.got)
		}
	})
	t.Run("reason required", func(t *testing.T) {
		_, _, r := adminSetup(t, nil, permReview)
		if w := testutil.Request(r, http.MethodPost, "/v1/admin/location-identities/7/revoke", map[string]any{}, nil); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", w.Code)
		}
	})
}

func TestAdmin_Districts(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "config.read")
		mock.ExpectQuery(`SELECT id, state_id, name, lgd_code, aliases, active FROM districts WHERE id > \? AND state_id = \? AND active = 1 AND name LIKE \? ORDER BY id LIMIT \?`).
			WithArgs(0, stateTN, "%than%", adminPageSize+1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "state_id", "name", "lgd_code", "aliases", "active"}).AddRow(thanjavur, stateTN, "Thanjavur", 600, `["Tanjore"]`, true))
		w := testutil.Request(r, http.MethodGet, "/v1/admin/geo/districts?stateId=23&q=than", nil, nil)
		var got []AdminDistrict
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
		if w.Code != http.StatusOK || len(got) != 1 || got[0].LgdCode == nil || *got[0].LgdCode != 600 || got[0].Aliases[0] != "Tanjore" {
			t.Fatalf("got %d %s", w.Code, w.Body)
		}
	})
	t.Run("create audits", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "geo.write")
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM states WHERE id = \?`).WithArgs(stateTN).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
		mock.ExpectExec(`INSERT INTO districts \(state_id, name, lgd_code, aliases, active\) VALUES \(\?, \?, \?, \?, 1\)`).
			WithArgs(stateTN, "Mayiladuthurai", 640, `["Mayavaram"]`).WillReturnResult(sqlmock.NewResult(700, 1))
		mock.ExpectExec(`INSERT INTO config_audit`).WithArgs("districts", "700", nil, sqlmock.AnyArg(), adminID).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(adminAuditRe).WithArgs(adminID, "geo.district_create", "district", "700", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		w := testutil.Request(r, http.MethodPost, "/v1/admin/geo/districts", map[string]any{"stateId": stateTN, "name": " Mayiladuthurai ", "lgdCode": 640, "aliases": []string{"Mayavaram", ""}}, nil)
		var got AdminDistrict
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
		if w.Code != http.StatusCreated || got.ID != 700 || !got.Active || len(got.Aliases) != 1 {
			t.Fatalf("got %d %s", w.Code, w.Body)
		}
	})
	t.Run("update deactivates", func(t *testing.T) {
		_, mock, r := adminSetup(t, nil, "geo.write")
		mock.ExpectBegin()
		mock.ExpectQuery(`FROM districts WHERE id = \? FOR UPDATE`).WithArgs(thanjavur).
			WillReturnRows(sqlmock.NewRows([]string{"id", "state_id", "name", "lgd_code", "aliases", "active"}).AddRow(thanjavur, stateTN, "Thanjavur", nil, `["Tanjore"]`, true))
		mock.ExpectExec(`UPDATE districts SET name = \?, lgd_code = \?, aliases = \?, active = \? WHERE id = \?`).
			WithArgs("Thanjavur", nil, `["Tanjore"]`, false, thanjavur).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO config_audit`).WithArgs("districts", "26", sqlmock.AnyArg(), sqlmock.AnyArg(), adminID).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(adminAuditRe).WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		w := testutil.Request(r, http.MethodPut, "/v1/admin/geo/districts/26", map[string]any{"active": false}, nil)
		var got AdminDistrict
		_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
		if w.Code != http.StatusOK || got.Active {
			t.Fatalf("got %d %s", w.Code, w.Body)
		}
	})
}

func TestAdmin_States(t *testing.T) {
	_, _, deny := adminSetup(t, nil, permReview)
	if w := testutil.Request(deny, http.MethodGet, "/v1/admin/geo/states", nil, nil); w.Code != http.StatusForbidden {
		t.Fatalf("without config.read: %d", w.Code)
	}
	_, mock, r := adminSetup(t, nil, permConfigRead)
	mock.ExpectQuery(`SELECT id, code, name, is_ut, lgd_code, active FROM states ORDER BY name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "name", "is_ut", "lgd_code", "active"}).
			AddRow(12, "KL", "Kerala", false, nil, false).
			AddRow(stateTN, "TN", "Tamil Nadu", false, 33, true))
	w := testutil.Request(r, http.MethodGet, "/v1/admin/geo/states", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got []AdminState
	_ = json.Unmarshal(decode(t, w.Body.Bytes()).Data, &got)
	if len(got) != 2 || got[0].Active || got[0].LgdCode != nil || got[1].Code != "TN" || got[1].LgdCode == nil || *got[1].LgdCode != 33 || !got[1].Active {
		t.Fatalf("body %s", w.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
