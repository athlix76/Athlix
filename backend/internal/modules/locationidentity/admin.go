package locationidentity

import (
	"context"
	"database/sql"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/userdir"
)

// auditFn writes an admin_audit_log row in the given querier (the handler binds the gin
// context so the admin id and IP come from the request).
type auditFn func(q db.Querier, action, targetType string, targetID int64, details any) error

var last4Re = regexp.MustCompile(`^[0-9]{4}$`)

func (s *Service) rejectionReasons(ctx context.Context) []string {
	return s.d.Settings.StringList(ctx, "location.rejection_reasons", defRejectionReasons)
}

func wrapInternal(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := httpx.As(err); ok {
		return err
	}
	return httpx.Internal().Wrap(err)
}

// ---- queue ----

// Queue is GET /admin/location-identities.
func (s *Service) Queue(ctx context.Context, f QueueFilter, p httpx.PageParams) ([]AdminLocationRow, string, error) {
	if f.Status == "" {
		f.Status = statusPending
	}
	v := map[string]string{}
	if !slices.Contains(allStatuses, f.Status) {
		v["status"] = "is not a valid status"
	}
	if f.IdentityType != "" && f.IdentityType != typeNative && f.IdentityType != typeResidence {
		v["identityType"] = "must be native or residence"
	}
	if len(v) > 0 {
		return nil, "", httpx.Validation(v)
	}
	rows, err := s.repo.queue(ctx, f, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	sums, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	out := make([]AdminLocationRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAdminRow(r, sums))
	}
	return out, next, nil
}

func toAdminRow(r adminRow, sums map[int64]userdir.Summary) AdminLocationRow {
	sum, ok := sums[r.UserID]
	if !ok {
		sum = userdir.Summary{ID: r.UserID}
	}
	return AdminLocationRow{
		ID: r.ID, User: sum, IsMinor: r.IsMinor, IdentityType: r.Type, District: r.district(), State: r.state(),
		ProofType: r.ProofType, ProofHolder: r.ProofHolder, Status: r.Status, SubmittedAt: r.SubmittedAt.UTC(),
	}
}

// ---- detail ----

// Detail is GET /admin/location-identities/:id. With sign=true it signs the proof URL and
// writes the location.proof_viewed audit row first (no audit row, no URL).
func (s *Service) Detail(ctx context.Context, id int64, sign bool, audit auditFn) (AdminLocationDetail, error) {
	r, err := s.repo.detail(ctx, id)
	if db.IsNoRows(err) {
		return AdminLocationDetail{}, httpx.NotFound("Location identity")
	}
	if err != nil {
		return AdminLocationDetail{}, httpx.Internal().Wrap(err)
	}
	hist, err := s.repo.history(ctx, r.UserID, r.Type, r.ID)
	if err != nil {
		return AdminLocationDetail{}, httpx.Internal().Wrap(err)
	}
	sums, err := s.d.Users.Summaries(ctx, []int64{r.UserID})
	if err != nil {
		return AdminLocationDetail{}, httpx.Internal().Wrap(err)
	}
	out := AdminLocationDetail{
		AdminLocationRow: toAdminRow(r, sums),
		ProofPurgedAt:    ptrTime(r.ProofPurgedAt),
		AadhaarLast4:     ptrStr(r.AadhaarLast4),
		RejectionCode:    ptrStr(r.RejectionCode),
		RejectionReason:  ptrStr(r.RejectionReason),
		ReviewNote:       ptrStr(r.ReviewNote),
		ReviewedAt:       ptrTime(r.ReviewedAt),
		VerifiedAt:       ptrTime(r.VerifiedAt),
		ExpiresAt:        ptrTime(r.ExpiresAt),
		ProfileDistrict:  ptrStr(r.ProfileDistrict),
		History:          make([]LocationIdentity, 0, len(hist)),
		Checklist: Checklist{
			UtilityBillMaxAgeDays: s.d.Settings.Int(ctx, "location.utility_bill_max_age_days", defUtilityBillMaxDays),
			RejectionReasons:      s.rejectionReasons(ctx),
		},
	}
	if r.ReviewedBy.Valid {
		out.ReviewedBy = &AdminRef{ID: r.ReviewedBy.Int64, Name: r.ReviewerName.String}
	}
	if r.ProofUploadID.Valid {
		out.ProofContentType = ptrStr(r.ProofContentType)
	}
	for _, h := range hist {
		out.History = append(out.History, h.toAPI())
	}
	if sign && r.ProofUploadID.Valid {
		if u := s.d.Media.ReviewerViewURL(ctx, r.ProofUploadID.Int64); u != "" {
			if err := audit(s.d.DB, "location.proof_viewed", "location_identity", r.ID, map[string]any{"uploadId": r.ProofUploadID.Int64}); err != nil {
				return AdminLocationDetail{}, httpx.Internal().Wrap(err)
			}
			out.ProofURL = &u
		}
	}
	return out, nil
}

// ---- approve / reject / revoke ----

// Approve verifies a pending row; the user's verified row of the same type becomes superseded.
func (s *Service) Approve(ctx context.Context, adminID, id int64, in approveBody, audit auditFn) (AdminLocationDetail, error) {
	last4 := ""
	if in.AadhaarLast4 != nil {
		last4 = strings.TrimSpace(*in.AadhaarLast4)
	}
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	if utf8.RuneCountInString(note) > maxNoteLen {
		return AdminLocationDetail{}, httpx.Validation(map[string]string{"note": "must be at most 255 characters"})
	}
	now := s.now()
	var row reviewRow
	var old *verifiedRef
	var expires *time.Time
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		row, err = lockForReview(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Location identity")
		}
		if err != nil {
			return err
		}
		if row.Status != statusPending {
			return httpx.InvalidState("Only a pending identity can be approved.")
		}
		if last4 != "" {
			if row.ProofType != proofMaskedAadhaar {
				return httpx.Validation(map[string]string{"aadhaarLast4": "is only for a masked Aadhaar proof"})
			}
			if !last4Re.MatchString(last4) {
				return httpx.Validation(map[string]string{"aadhaarLast4": "must be exactly 4 digits"})
			}
		}
		if row.Type == typeResidence {
			t := now.AddDate(0, s.d.Settings.Int(ctx, "location.residence_validity_months", defValidityMonths), 0)
			expires = &t
		}
		// Supersede the old verified row FIRST so the generated verified_slot UNIQUE key never sees two.
		old, err = lockVerified(ctx, tx, row.UserID, row.Type)
		if err != nil {
			return err
		}
		if old != nil {
			if _, err := setStatus(ctx, tx, old.ID, statusVerified, statusSuperseded); err != nil {
				return err
			}
		}
		if err := approveRow(ctx, tx, id, adminID, now, expires, last4, note); err != nil {
			return err
		}
		details := map[string]any{"identityType": row.Type, "withNote": note != ""}
		if old != nil {
			details["supersededId"] = old.ID
		}
		return audit(tx, "location.approve", "location_identity", id, details)
	})
	if err != nil {
		return AdminLocationDetail{}, wrapInternal(err)
	}
	if old != nil {
		s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: old.ID, UserID: row.UserID, IdentityType: row.Type, DistrictID: old.DistrictID, Status: statusSuperseded, PrevStatus: statusVerified})
	}
	s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: id, UserID: row.UserID, IdentityType: row.Type, DistrictID: row.DistrictID, Status: statusVerified, PrevStatus: statusPending, ExpiresAt: fmtTime(expires)})
	return s.Detail(ctx, id, false, audit)
}

// Reject rejects a pending row; unmasked_aadhaar deletes the proof at once.
func (s *Service) Reject(ctx context.Context, adminID, id int64, in rejectBody, audit auditFn) (AdminLocationDetail, error) {
	code := strings.TrimSpace(in.RejectionCode)
	reason := ""
	if in.RejectionReason != nil {
		reason = strings.TrimSpace(*in.RejectionReason)
	}
	v := map[string]string{}
	if !slices.Contains(s.rejectionReasons(ctx), code) {
		v["rejectionCode"] = "is not a valid rejection reason"
	}
	if utf8.RuneCountInString(reason) > maxNoteLen {
		v["rejectionReason"] = "must be at most 255 characters"
	}
	if len(v) > 0 {
		return AdminLocationDetail{}, httpx.Validation(v)
	}
	now := s.now()
	var row reviewRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		row, err = lockForReview(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Location identity")
		}
		if err != nil {
			return err
		}
		if row.Status != statusPending {
			return httpx.InvalidState("Only a pending identity can be rejected.")
		}
		if err := rejectRow(ctx, tx, id, adminID, now, code, reason); err != nil {
			return err
		}
		return audit(tx, "location.reject", "location_identity", id, map[string]any{"rejectionCode": code})
	})
	if err != nil {
		return AdminLocationDetail{}, wrapInternal(err)
	}
	if code == rejectUnmaskedAadhaar && row.ProofUploadID.Valid {
		s.purgeNow(ctx, id, row.ProofUploadID.Int64)
	}
	s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: id, UserID: row.UserID, IdentityType: row.Type, DistrictID: row.DistrictID, Status: statusRejected, PrevStatus: statusPending, RejectionCode: code})
	return s.Detail(ctx, id, false, audit)
}

// Revoke moves a verified row to revoked (e.g. fraud).
func (s *Service) Revoke(ctx context.Context, id int64, in revokeBody, audit auditFn) (AdminLocationDetail, error) {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return AdminLocationDetail{}, httpx.Validation(map[string]string{"reason": "is required"})
	}
	if utf8.RuneCountInString(reason) > maxNoteLen {
		return AdminLocationDetail{}, httpx.Validation(map[string]string{"reason": "must be at most 255 characters"})
	}
	var row reviewRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		row, err = lockForReview(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("Location identity")
		}
		if err != nil {
			return err
		}
		if row.Status != statusVerified {
			return httpx.InvalidState("Only a verified identity can be revoked.")
		}
		if err := revokeRow(ctx, tx, id, reason); err != nil {
			return err
		}
		return audit(tx, "location.revoke", "location_identity", id, map[string]any{"identityType": row.Type})
	})
	if err != nil {
		return AdminLocationDetail{}, wrapInternal(err)
	}
	s.publishChanged(ctx, 0, events.LocationIdentityPayload{IdentityID: id, UserID: row.UserID, IdentityType: row.Type, DistrictID: row.DistrictID, Status: statusRevoked, PrevStatus: statusVerified})
	return s.Detail(ctx, id, false, audit)
}

// ---- districts ----

// AdminStates is GET /admin/geo/states: every state (inactive too), by name.
func (s *Service) AdminStates(ctx context.Context) ([]AdminState, error) {
	out, err := s.repo.adminStates(ctx)
	if err != nil {
		return nil, httpx.Internal().Wrap(err)
	}
	return out, nil
}

// AdminDistricts is GET /admin/geo/districts.
func (s *Service) AdminDistricts(ctx context.Context, f DistrictFilter, p httpx.PageParams) ([]AdminDistrict, string, error) {
	f.Q = strings.TrimSpace(f.Q)
	if utf8.RuneCountInString(f.Q) > maxDistrictName {
		return nil, "", httpx.Validation(map[string]string{"q": "must be at most 80 characters"})
	}
	rows, err := s.repo.adminDistricts(ctx, f, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out := make([]AdminDistrict, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toAdmin())
	}
	return out, next, nil
}

func cleanAliases(in []string, v map[string]string) []string {
	out := []string{}
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" || slices.Contains(out, a) {
			continue
		}
		if utf8.RuneCountInString(a) > maxDistrictName {
			v["aliases"] = "each alias must be at most 80 characters"
		}
		out = append(out, a)
	}
	if len(out) > maxAliases {
		v["aliases"] = "at most 20 aliases"
	}
	return out
}

func validName(name string, v map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		v["name"] = "is required"
	} else if utf8.RuneCountInString(name) > maxDistrictName {
		v["name"] = "must be at most 80 characters"
	}
	return name
}

func auditDistrict(d districtRow) map[string]any {
	return map[string]any{"stateId": d.StateID, "name": d.Name, "lgdCode": nullInt(d.LgdCode), "aliases": d.Aliases, "active": d.Active}
}

// bumpPlaces drops the places cache here and tells the other instances (platform/eligibility
// Places.Bump). A Redis error only delays them to the cache TTL; it never fails the edit.
func (s *Service) bumpPlaces(ctx context.Context) {
	if err := s.d.Eligibility.Places().Bump(ctx); err != nil {
		logger.From(ctx).Warn("places version bump failed; other instances reload at the cache TTL", slog.String("err", err.Error()))
	}
}

// CreateDistrict is POST /admin/geo/districts.
func (s *Service) CreateDistrict(ctx context.Context, adminID int64, in districtCreateBody, audit auditFn) (AdminDistrict, error) {
	v := map[string]string{}
	row := districtRow{StateID: in.StateID, Active: true}
	row.Name = validName(in.Name, v)
	row.Aliases = cleanAliases(in.Aliases, v)
	if in.StateID <= 0 {
		v["stateId"] = "is required"
	}
	if in.LgdCode != nil {
		if *in.LgdCode <= 0 {
			v["lgdCode"] = "must be a positive number"
		}
		row.LgdCode = sql.NullInt64{Int64: *in.LgdCode, Valid: true}
	}
	if len(v) > 0 {
		return AdminDistrict{}, httpx.Validation(v)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		ok, err := stateExists(ctx, tx, in.StateID)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.Validation(map[string]string{"stateId": "is not a known state"})
		}
		row.ID, err = insertDistrict(ctx, tx, row)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A district with this name or LGD code already exists.")
		}
		if err != nil {
			return err
		}
		if err := audit.config(ctx, tx, row.ID, nil, auditDistrict(row), adminID); err != nil {
			return err
		}
		return audit(tx, "geo.district_create", "district", row.ID, map[string]any{"stateId": row.StateID})
	})
	if err != nil {
		return AdminDistrict{}, wrapInternal(err)
	}
	s.bumpPlaces(ctx)
	return row.toAdmin(), nil
}

// UpdateDistrict is PUT /admin/geo/districts/:id.
func (s *Service) UpdateDistrict(ctx context.Context, adminID, id int64, in districtUpdateBody, audit auditFn) (AdminDistrict, error) {
	v := map[string]string{}
	var name string
	if in.Name != nil {
		name = validName(*in.Name, v)
	}
	var aliases []string
	if in.Aliases != nil {
		aliases = cleanAliases(*in.Aliases, v)
	}
	if in.LgdCode != nil && *in.LgdCode <= 0 {
		v["lgdCode"] = "must be a positive number"
	}
	if len(v) > 0 {
		return AdminDistrict{}, httpx.Validation(v)
	}
	var row districtRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		old, err := lockDistrict(ctx, tx, id)
		if db.IsNoRows(err) {
			return httpx.NotFound("District")
		}
		if err != nil {
			return err
		}
		row = old
		if in.Name != nil {
			row.Name = name
		}
		if in.Aliases != nil {
			row.Aliases = aliases
		}
		if in.LgdCode != nil {
			row.LgdCode = sql.NullInt64{Int64: *in.LgdCode, Valid: true}
		}
		if in.Active != nil {
			row.Active = *in.Active
		}
		err = updateDistrict(ctx, tx, row)
		if db.IsDuplicate(err) {
			return httpx.Conflict("A district with this name or LGD code already exists.")
		}
		if err != nil {
			return err
		}
		if err := audit.config(ctx, tx, id, auditDistrict(old), auditDistrict(row), adminID); err != nil {
			return err
		}
		return audit(tx, "geo.district_update", "district", id, map[string]any{"stateId": row.StateID, "active": row.Active})
	})
	if err != nil {
		return AdminDistrict{}, wrapInternal(err)
	}
	s.bumpPlaces(ctx)
	return row.toAdmin(), nil
}
