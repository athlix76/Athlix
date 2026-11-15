package organiser

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	pmedia "athlix/backend/internal/platform/media"
)

// Service holds the organiser verification logic.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// editable lists the statuses in which the organiser may change and submit the checklist.
var editable = []string{StatusNotStarted, StatusDraft, StatusRejected}

// listStatuses are the ?status= values of the admin list.
var listStatuses = []string{StatusDraft, StatusSubmitted, StatusApproved, StatusRejected}

// Get is GET /organiser/verification for userID.
func (s *Service) Get(ctx context.Context, userID int64) (Verification, error) {
	p, err := s.repo.profile(ctx, s.d.DB, userID, false)
	if err != nil {
		return Verification{}, err
	}
	steps, err := s.repo.steps(ctx, s.d.DB, userID)
	if err != nil {
		return Verification{}, err
	}
	return s.build(ctx, p, steps), nil
}

// build turns the rows into the API shape and signs the document links. The caller has already
// checked that the viewer may see this organiser's documents (the organiser or a reviewer).
func (s *Service) build(ctx context.Context, p profileRow, steps []stepRow) Verification {
	ids := []int64{}
	for _, st := range steps {
		if st.HasSub && st.UploadID != nil {
			ids = append(ids, *st.UploadID)
		}
	}
	urls := s.d.Media.ViewURLs(ctx, ids)
	out := Verification{Status: p.Status, OrgName: p.OrgName, OrgType: p.OrgType, ReviewNote: p.ReviewNote,
		SubmittedAt: p.SubmittedAt, ReviewedAt: p.ReviewedAt, Steps: make([]Step, 0, len(steps))}
	for _, st := range steps {
		step := Step{StepID: st.ID, StepCode: st.Code, Title: st.Title, Description: st.Description,
			InputType: st.InputType, IsRequired: st.Required && st.Active}
		if st.HasSub {
			sub := &Submission{Status: st.SubStatus, ReviewNote: st.SubNote}
			switch st.InputType {
			case inputText:
				sub.ValueText = st.ValueText
			case inputCheckbox:
				checked := st.ValueText != nil && *st.ValueText == "true"
				sub.Checked = &checked
			case inputDocument:
				if st.UploadID != nil {
					ref := &DocRef{UploadID: *st.UploadID}
					if u, ok := urls[*st.UploadID]; ok {
						ref.URL = &u
					}
					sub.Document = ref
				}
			}
			step.Submission = sub
		}
		out.Steps = append(out.Steps, step)
	}
	return out
}

// stepOp is one validated change to a submission.
type stepOp struct {
	stepID int64
	remove bool
	value  *string
	upload *int64
	field  string // request field for claim errors
}

// Save is PUT /organiser/verification.
func (s *Service) Save(ctx context.Context, userID int64, in saveRequest) (Verification, error) {
	fields := map[string]string{}
	var name *string
	if in.OrgName != nil {
		v := strings.TrimSpace(*in.OrgName)
		switch {
		case utf8.RuneCountInString(v) > maxOrgName:
			fields["orgName"] = "must be at most 120 characters"
		case hasControl(v, false):
			fields["orgName"] = "contains characters that are not allowed"
		case v != "":
			name = &v
		}
	}
	if in.OrgType != nil && !slices.Contains(orgTypes, *in.OrgType) {
		fields["orgType"] = "must be one of " + strings.Join(orgTypes, ", ")
	}
	seen := map[int64]bool{}
	for i, st := range in.Steps {
		key := "steps[" + strconv.Itoa(i) + "].stepId"
		if st.StepID <= 0 {
			fields[key] = "is required"
		} else if seen[st.StepID] {
			fields[key] = "is listed twice"
		}
		seen[st.StepID] = true
	}
	if len(fields) > 0 {
		return Verification{}, httpx.Validation(fields)
	}

	var replaced []int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		p, err := s.repo.profile(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if !slices.Contains(editable, p.Status) {
			return httpx.InvalidState("Your verification is " + p.Status + " and can no longer be edited.")
		}
		types, err := s.repo.activeSteps(ctx, tx)
		if err != nil {
			return err
		}
		docs, err := s.repo.documents(ctx, tx, userID)
		if err != nil {
			return err
		}
		ops := make([]stepOp, 0, len(in.Steps))
		for i, st := range in.Steps {
			op, msgs := validateStep(i, st, types)
			for k, v := range msgs {
				fields[k] = v
			}
			if len(msgs) == 0 {
				ops = append(ops, op)
			}
		}
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}

		if in.OrgName == nil {
			name = p.OrgName
		}
		typ := p.OrgType
		if in.OrgType != nil {
			typ = in.OrgType
		}
		if err := s.repo.saveProfile(ctx, tx, userID, name, typ); err != nil {
			return err
		}
		for _, op := range ops {
			old := docs[op.stepID]
			if op.remove {
				if err := s.repo.deleteSubmission(ctx, tx, userID, op.stepID); err != nil {
					return err
				}
				if old != 0 {
					replaced = append(replaced, old)
				}
				continue
			}
			if op.upload != nil {
				if err := s.d.Media.Claim(ctx, tx, userID, *op.upload, pmedia.PurposeOrganiserDocument); err != nil {
					return claimErr(err, op.field)
				}
				if old != 0 && old != *op.upload {
					replaced = append(replaced, old)
				}
			}
			if err := s.repo.upsertSubmission(ctx, tx, userID, op.stepID, op.value, op.upload); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Verification{}, err
	}
	for _, id := range replaced {
		if err := s.d.Media.Purge(ctx, id); err != nil {
			logger.From(ctx).Warn("purge replaced organiser document", slog.Int64("userId", userID), slog.Int64("uploadId", id), slog.String("err", err.Error()))
		}
	}
	return s.Get(ctx, userID)
}

// validateStep checks one step entry against the active step's input type.
func validateStep(i int, st stepInput, types map[int64]string) (stepOp, map[string]string) {
	prefix := "steps[" + strconv.Itoa(i) + "]."
	typ, ok := types[st.StepID]
	if !ok {
		return stepOp{}, map[string]string{prefix + "stepId": "is not an active step"}
	}
	op := stepOp{stepID: st.StepID}
	switch typ {
	case inputText:
		if st.ValueText == nil {
			return op, map[string]string{prefix + "valueText": "is required for a text step"}
		}
		v := strings.TrimSpace(*st.ValueText)
		if utf8.RuneCountInString(v) > maxValue {
			return op, map[string]string{prefix + "valueText": "must be at most 500 characters"}
		}
		if hasControl(v, true) {
			return op, map[string]string{prefix + "valueText": "contains characters that are not allowed"}
		}
		if v == "" {
			op.remove = true
		} else {
			op.value = &v
		}
	case inputDocument:
		if st.DocumentUploadID == nil || *st.DocumentUploadID < 0 {
			return op, map[string]string{prefix + "documentUploadId": "is required for a document step"}
		}
		if *st.DocumentUploadID == 0 {
			op.remove = true
		} else {
			op.upload, op.field = st.DocumentUploadID, prefix+"documentUploadId"
		}
	case inputCheckbox:
		if st.Checked == nil {
			return op, map[string]string{prefix + "checked": "is required for a checkbox step"}
		}
		if *st.Checked {
			v := "true"
			op.value = &v
		} else {
			op.remove = true
		}
	default:
		return op, map[string]string{prefix + "stepId": "is not an active step"}
	}
	return op, nil
}

// Submit is POST /organiser/verification/submit.
func (s *Service) Submit(ctx context.Context, userID int64) (Verification, error) {
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		p, err := s.repo.profile(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if !slices.Contains(editable, p.Status) {
			return httpx.InvalidState("Your verification is already " + p.Status + ".")
		}
		fields := map[string]string{}
		if p.OrgName == nil {
			fields["orgName"] = "is required"
		}
		if p.OrgType == nil {
			fields["orgType"] = "is required"
		}
		missing, err := s.repo.missingRequired(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, code := range missing {
			fields["steps."+code] = "is required"
		}
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
		return s.repo.markSubmitted(ctx, tx, userID, s.d.Now().UTC())
	})
	if err != nil {
		return Verification{}, err
	}
	return s.Get(ctx, userID)
}

// parseStatus validates the admin list ?status= (default submitted).
func parseStatus(v string) (string, error) {
	if v == "" {
		return StatusSubmitted, nil
	}
	if !slices.Contains(listStatuses, v) {
		return "", httpx.Validation(map[string]string{"status": "must be one of " + strings.Join(listStatuses, ", ")})
	}
	return v, nil
}

// AdminList is GET /admin/organisers.
func (s *Service) AdminList(ctx context.Context, status string, page httpx.PageParams) ([]AdminRow, string, error) {
	rows, err := s.repo.adminList(ctx, status, page.Cursor.ID, page.Limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].User.ID})
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.User.ID
	}
	cards, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]AdminRow, 0, len(rows))
	for _, r := range rows {
		card, ok := cards[r.User.ID]
		if !ok {
			continue
		}
		r.User = card
		out = append(out, r)
	}
	return out, next, nil
}

// AdminDetail is GET /admin/organisers/:userId.
func (s *Service) AdminDetail(ctx context.Context, userID int64) (AdminVerification, error) {
	card, ok, err := s.d.Users.Summary(ctx, userID)
	if err != nil {
		return AdminVerification{}, err
	}
	if !ok {
		return AdminVerification{}, httpx.NotFound("Organiser")
	}
	p, err := s.repo.profile(ctx, s.d.DB, userID, false)
	if err != nil {
		return AdminVerification{}, err
	}
	if !p.exists {
		return AdminVerification{}, httpx.NotFound("Organiser")
	}
	steps, err := s.repo.steps(ctx, s.d.DB, userID)
	if err != nil {
		return AdminVerification{}, err
	}
	return AdminVerification{User: card, Verification: s.build(ctx, p, steps)}, nil
}

// Review approves or rejects a submitted verification. onTx writes the audit row in the same
// transaction; the event is published after the commit.
func (s *Service) Review(ctx context.Context, userID, adminID int64, approve bool, note *string, onTx func(tx *sql.Tx) error) (AdminVerification, error) {
	u, err := s.d.Users.Get(ctx, userID)
	if db.IsNoRows(err) || (err == nil && u.Status == "deleted") {
		return AdminVerification{}, httpx.NotFound("Organiser")
	}
	if err != nil {
		return AdminVerification{}, err
	}
	status, name := StatusRejected, events.OrganiserRejected
	if approve {
		status, name = StatusApproved, events.OrganiserApproved
	}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		p, err := s.repo.profile(ctx, tx, userID, true)
		if err != nil {
			return err
		}
		if !p.exists {
			return httpx.NotFound("Organiser")
		}
		if p.Status != StatusSubmitted {
			return httpx.InvalidState("Only a submitted verification can be reviewed; this one is " + p.Status + ".")
		}
		if err := s.repo.review(ctx, tx, userID, adminID, status, note, s.d.Now().UTC()); err != nil {
			return err
		}
		return onTx(tx)
	})
	if err != nil {
		return AdminVerification{}, err
	}
	payload := events.OrganiserPayload{UserID: userID}
	if !approve && note != nil {
		payload.Reason = *note
	}
	if err := s.d.Events.Publish(ctx, events.New(name, adminID, payload)); err != nil {
		logger.From(ctx).Error("publish organiser review", slog.Int64("userId", userID), slog.String("event", name), slog.String("err", err.Error()))
	}
	logger.From(ctx).Info("organiser reviewed", slog.Int64("userId", userID), slog.Int64("adminId", adminID), slog.String("status", status))
	return s.AdminDetail(ctx, userID)
}

// onUserDeleted purges the organiser documents and clears the stored details (idempotent).
func (s *Service) onUserDeleted(ctx context.Context, e events.Event) error {
	var p events.UserPayload
	if err := e.Decode(&p); err != nil || p.UserID == 0 {
		return nil // malformed: nothing to retry
	}
	// Files first: Purge is idempotent, so a retry after a failed wipe still finds the ids.
	docs, err := s.repo.allDocuments(ctx, p.UserID)
	if err != nil {
		return err
	}
	for _, id := range docs {
		if err := s.d.Media.Purge(ctx, id); err != nil {
			return err
		}
	}
	if err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error { return s.repo.wipe(ctx, tx, p.UserID) }); err != nil {
		return err
	}
	logger.From(ctx).Info("organiser data wiped", slog.Int64("userId", p.UserID), slog.Int("documents", len(docs)))
	return nil
}

func claimErr(err error, field string) error {
	switch {
	case errors.Is(err, pmedia.ErrNotFound):
		return httpx.Validation(map[string]string{field: "is not one of your organiser document uploads"})
	case errors.Is(err, pmedia.ErrNotUploaded):
		return httpx.Validation(map[string]string{field: "has not been uploaded yet"})
	case errors.Is(err, pmedia.ErrMismatch):
		return httpx.Validation(map[string]string{field: "does not match the declared file; upload it again"})
	}
	return err
}

// hasControl reports control characters; newlines and tabs are allowed in multi-line text.
func hasControl(s string, multiline bool) bool {
	for _, r := range s {
		if multiline && (r == '\n' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// validNote trims an admin note; required notes must be non-empty. At most 500 characters.
func validNote(n *string, required bool) (*string, error) {
	if n != nil {
		v := strings.TrimSpace(*n)
		if utf8.RuneCountInString(v) > maxNote {
			return nil, httpx.Validation(map[string]string{"note": "must be at most 500 characters"})
		}
		if v != "" {
			return &v, nil
		}
	}
	if required {
		return nil, httpx.Validation(map[string]string{"note": "is required"})
	}
	return nil, nil
}
