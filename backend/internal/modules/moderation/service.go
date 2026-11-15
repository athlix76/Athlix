package moderation

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Service holds the moderation rules.
type Service struct {
	d    *app.Deps
	repo *repo
}

func newService(d *app.Deps) *Service { return &Service{d: d, repo: &repo{db: d.DB}} }

// eventRemovedReason is the organiser-visible text on event.cancelled for a moderation removal.
// The admin's own note stays internal.
const eventRemovedReason = "Removed by Athlix moderation."

func (s *Service) threshold(ctx context.Context) int {
	return s.d.Settings.Int(ctx, "moderation.auto_hide_report_threshold", defaultAutoHideThreshold)
}

// reportRateLimit is reports per user per hour; below 1 falls back to the default (the limit
// cannot be switched off from config).
func (s *Service) reportRateLimit(ctx context.Context) int {
	n := s.d.Settings.Int(ctx, "moderation.report_rate_limit_per_hour", defaultReportRateLimit)
	if n < 1 {
		return defaultReportRateLimit
	}
	return n
}

// CreateReport files a report. created=false means the caller already reported the target.
func (s *Service) CreateReport(ctx context.Context, reporterID int64, in CreateReportInput) (ReportRef, bool, error) {
	if err := validateReport(reporterID, &in); err != nil {
		return ReportRef{}, false, err
	}
	ok, err := s.d.Limiter.Allow(ctx, "report:"+strconv.FormatInt(reporterID, 10), s.reportRateLimit(ctx), reportRateWindow)
	if err != nil {
		return ReportRef{}, false, httpx.Internal().Wrap(err)
	}
	if !ok {
		return ReportRef{}, false, httpx.RateLimited("Too many reports. Please try again later.")
	}
	visible, err := s.repo.targetVisible(ctx, in.TargetType, reporterID, in.TargetID)
	if err != nil {
		return ReportRef{}, false, httpx.Internal().Wrap(err)
	}
	if visible && in.TargetType == TargetUser {
		blocked, err := s.d.Users.BlockedEitherWay(ctx, reporterID, in.TargetID)
		if err != nil {
			return ReportRef{}, false, httpx.Internal().Wrap(err)
		}
		visible = !blocked
	}
	if !visible {
		return ReportRef{}, false, httpx.NotFound("Target")
	}
	if id, found, err := s.repo.findReport(ctx, s.d.DB, reporterID, in.TargetType, in.TargetID); err != nil {
		return ReportRef{}, false, httpx.Internal().Wrap(err)
	} else if found {
		return ReportRef{ID: id}, false, nil
	}

	var id int64
	var hidden bool
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if id, err = s.repo.insertReport(ctx, tx, reporterID, in); err != nil {
			return err
		}
		hidden, err = s.maybeAutoHide(ctx, tx, in.TargetType, in.TargetID, id)
		return err
	})
	if db.IsDuplicate(err) {
		existing, found, ferr := s.repo.findReport(ctx, s.d.DB, reporterID, in.TargetType, in.TargetID)
		if ferr == nil && found {
			return ReportRef{ID: existing}, false, nil
		}
	}
	if err != nil {
		return ReportRef{}, false, httpx.Internal().Wrap(err)
	}
	log := logger.From(ctx)
	log.Info("report created", slog.Int64("reportId", id), slog.String("targetType", in.TargetType), slog.Int64("targetId", in.TargetID))
	if hidden {
		log.Info("target auto-hidden", slog.String("targetType", in.TargetType), slog.Int64("targetId", in.TargetID))
	}
	s.publish(ctx, events.New(events.ReportCreated, reporterID, events.ReportPayload{ReportID: id, TargetType: in.TargetType, TargetID: in.TargetID}))
	return ReportRef{ID: id}, true, nil
}

func validateReport(reporterID int64, in *CreateReportInput) error {
	f := map[string]string{}
	if !targetTypes[in.TargetType] {
		f["targetType"] = "must be one of " + strings.Join(targetTypeList, ", ")
	}
	if in.TargetID <= 0 {
		f["targetId"] = "is required"
	} else if in.TargetType == TargetUser && in.TargetID == reporterID {
		f["targetId"] = "you cannot report yourself"
	}
	if !reasons[in.Reason] {
		f["reason"] = "is not a valid reason"
	}
	if in.Details != nil {
		d := strings.TrimSpace(*in.Details)
		if utf8.RuneCountInString(d) > maxDetailsLen {
			f["details"] = "must be at most 1000 characters"
		}
		if d == "" {
			in.Details = nil
		} else {
			in.Details = &d
		}
	}
	if len(f) > 0 {
		return httpx.Validation(f)
	}
	return nil
}

// maybeAutoHide hides a post/comment/message once its open reports reach the threshold.
// A threshold below 1 disables auto-hide. Users, events, conversations and communities are only
// queued (no reversible hidden state, see autoHideable).
func (s *Service) maybeAutoHide(ctx context.Context, tx *sql.Tx, targetType string, targetID, reportID int64) (bool, error) {
	if !autoHideable[targetType] {
		return false, nil
	}
	limit := s.threshold(ctx)
	if limit < 1 {
		return false, nil
	}
	n, err := s.repo.countOpen(ctx, tx, targetType, targetID)
	if err != nil || n < limit {
		return false, err
	}
	changed, err := s.repo.autoHide(ctx, tx, targetType, targetID)
	if err != nil || !changed {
		return false, err
	}
	_, err = s.repo.insertAction(ctx, tx, 0, targetType, targetID, ActionAutoHide, "Open reports reached "+strconv.Itoa(n), reportID)
	return err == nil, err
}

// effect is what must happen after commit.
type effect struct {
	cancelled      *events.EventPayload
	invalidateUser int64
	actionID       int64
}

type transition struct {
	to   string
	from []string
}

// transitionFor maps (target type, action) to the status change. ok=false: not supported.
func transitionFor(targetType, action string) (transition, bool) {
	switch targetType {
	case TargetPost, TargetComment, TargetMessage:
		switch action {
		case ActionHide:
			return transition{"hidden", []string{"visible"}}, true
		case ActionUnhide:
			return transition{"visible", []string{"hidden"}}, true
		case ActionRemove:
			return transition{"deleted", []string{"visible", "hidden"}}, true
		}
	case TargetConversation, TargetCommunity:
		switch action {
		case ActionHide, ActionRemove:
			return transition{"removed", []string{"active", "archived"}}, true
		case ActionUnhide:
			return transition{"active", []string{"removed"}}, true
		}
	case TargetEvent:
		switch action {
		case ActionHide, ActionRemove:
			return transition{"removed", []string{"published", "pending_review"}}, true
		}
	case TargetUser:
		switch action {
		case ActionHide, ActionRemove, ActionSuspend:
			return transition{"suspended", []string{"active"}}, true
		case ActionUnhide, ActionUnsuspend:
			return transition{"active", []string{"suspended"}}, true
		}
	}
	return transition{}, false
}

// apply performs one action inside tx and records it. suspend/unsuspend on a non-user target
// acts on (and is recorded against) the target's owner.
func (s *Service) apply(ctx context.Context, tx *sql.Tx, adminID int64, targetType string, targetID int64, action, reason string, reportID int64) (effect, error) {
	var ef effect
	st, err := s.repo.lockTarget(ctx, tx, targetType, targetID)
	if db.IsNoRows(err) {
		return ef, httpx.NotFound("Target")
	}
	if err != nil {
		return ef, err
	}
	if (action == ActionSuspend || action == ActionUnsuspend) && targetType != TargetUser {
		if st.OwnerID == 0 {
			return ef, httpx.InvalidState("This item has no owner to suspend.")
		}
		targetType, targetID = TargetUser, st.OwnerID
		if st, err = s.repo.lockTarget(ctx, tx, TargetUser, targetID); db.IsNoRows(err) {
			return ef, httpx.NotFound("Target")
		} else if err != nil {
			return ef, err
		}
	}
	if action != ActionWarn {
		tr, ok := transitionFor(targetType, action)
		if !ok {
			return ef, httpx.InvalidState("This action is not supported for this item.")
		}
		if st.Status != tr.to {
			if !contains(tr.from, st.Status) {
				return ef, httpx.InvalidState("This action is not allowed in the item's current status.")
			}
			if err := s.repo.setTargetStatus(ctx, tx, targetType, targetID, tr.to); err != nil {
				return ef, err
			}
			switch targetType {
			case TargetUser:
				ef.invalidateUser = targetID
			case TargetEvent:
				ef.cancelled = &events.EventPayload{EventID: targetID, OrganiserID: st.OwnerID, SportID: st.SportID, Reason: eventRemovedReason}
			}
		}
	}
	ef.actionID, err = s.repo.insertAction(ctx, tx, adminID, targetType, targetID, action, reason, reportID)
	return ef, err
}

func (s *Service) after(ctx context.Context, adminID int64, ef effect) {
	if ef.invalidateUser > 0 {
		s.d.Users.Invalidate(ef.invalidateUser)
	}
	if ef.cancelled != nil {
		s.publish(ctx, events.New(events.EventCancelled, adminID, *ef.cancelled))
	}
}

func (s *Service) publish(ctx context.Context, e events.Event) {
	if err := s.d.Events.Publish(ctx, e); err != nil {
		logger.From(ctx).Error("publish event", slog.String("type", e.Type), slog.String("err", err.Error()))
	}
}

// Resolve closes a report with an action.
func (s *Service) Resolve(c *gin.Context, reportID int64, in ResolveInput) (AdminReport, error) {
	ctx := c.Request.Context()
	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}
	f := map[string]string{}
	if !resolveActions[in.Action] {
		f["action"] = "must be one of dismiss, hide, remove, suspend_user, warn"
	}
	if utf8.RuneCountInString(note) > maxNoteLen {
		f["note"] = "must be at most 500 characters"
	}
	if len(f) > 0 {
		return AdminReport{}, httpx.Validation(f)
	}
	adminID := authn.AdminID(c)
	now := s.d.Now().UTC()
	var ef effect
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		rep, err := s.repo.getReport(ctx, tx, reportID, true)
		if db.IsNoRows(err) {
			return httpx.NotFound("Report")
		}
		if err != nil {
			return err
		}
		if rep.Status != StatusOpen {
			return httpx.InvalidState("This report is already resolved.")
		}
		if in.Action == ActionDismiss {
			if err := s.repo.setReportStatus(ctx, tx, reportID, StatusDismissed, adminID, note, now); err != nil {
				return err
			}
			if err := s.liftAutoHide(ctx, tx, adminID, rep); err != nil {
				return err
			}
		} else {
			if ef, err = s.apply(ctx, tx, adminID, rep.TargetType, rep.TargetID, in.Action, note, reportID); err != nil {
				return err
			}
			if err := s.repo.actionOpenReports(ctx, tx, rep.TargetType, rep.TargetID, adminID, note, now); err != nil {
				return err
			}
		}
		return audit.Admin(c, tx, "report.resolve", "report", strconv.FormatInt(reportID, 10),
			map[string]any{"action": in.Action, "targetType": rep.TargetType, "targetId": rep.TargetID})
	})
	if err != nil {
		return AdminReport{}, asHTTP(err)
	}
	s.after(ctx, adminID, ef)
	rep, err := s.repo.getReport(ctx, s.d.DB, reportID, false)
	if err != nil {
		return AdminReport{}, httpx.Internal().Wrap(err)
	}
	out, err := s.buildReports(c, []reportRow{rep})
	if err != nil {
		return AdminReport{}, httpx.Internal().Wrap(err)
	}
	return out[0], nil
}

// liftAutoHide restores an auto-hidden item when a dismissal drops its open reports below the
// threshold and no admin has acted on it since the automatic hide.
func (s *Service) liftAutoHide(ctx context.Context, tx *sql.Tx, adminID int64, rep reportRow) error {
	if !autoHideable[rep.TargetType] {
		return nil
	}
	n, err := s.repo.countOpen(ctx, tx, rep.TargetType, rep.TargetID)
	if err != nil {
		return err
	}
	if limit := s.threshold(ctx); limit >= 1 && n >= limit {
		return nil
	}
	auto, err := s.repo.lastActionIsAutoHide(ctx, tx, rep.TargetType, rep.TargetID)
	if err != nil || !auto {
		return err
	}
	st, err := s.repo.lockTarget(ctx, tx, rep.TargetType, rep.TargetID)
	if db.IsNoRows(err) {
		return nil
	}
	if err != nil || st.Status != "hidden" {
		return err
	}
	if err := s.repo.setTargetStatus(ctx, tx, rep.TargetType, rep.TargetID, "visible"); err != nil {
		return err
	}
	_, err = s.repo.insertAction(ctx, tx, adminID, rep.TargetType, rep.TargetID, ActionUnhide, "Auto-hide lifted after dismissal", rep.ID)
	return err
}

// CreateAction applies a moderation action directly (no report).
func (s *Service) CreateAction(c *gin.Context, in ActionInput) (ModerationAction, error) {
	ctx := c.Request.Context()
	in.Reason = strings.TrimSpace(in.Reason)
	f := map[string]string{}
	if !targetTypes[in.TargetType] {
		f["targetType"] = "must be one of " + strings.Join(targetTypeList, ", ")
	}
	if in.TargetID <= 0 {
		f["targetId"] = "is required"
	}
	if !directActions[in.Action] {
		f["action"] = "must be one of hide, unhide, remove, suspend_user, unsuspend_user, warn"
	}
	if in.Reason == "" {
		f["reason"] = "is required"
	} else if utf8.RuneCountInString(in.Reason) > maxNoteLen {
		f["reason"] = "must be at most 500 characters"
	}
	if len(f) > 0 {
		return ModerationAction{}, httpx.Validation(f)
	}
	adminID := authn.AdminID(c)
	now := s.d.Now().UTC()
	var ef effect
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if ef, err = s.apply(ctx, tx, adminID, in.TargetType, in.TargetID, in.Action, in.Reason, 0); err != nil {
			return err
		}
		if in.Action == ActionHide || in.Action == ActionRemove || in.Action == ActionSuspend {
			if err := s.repo.actionOpenReports(ctx, tx, in.TargetType, in.TargetID, adminID, in.Reason, now); err != nil {
				return err
			}
		}
		return audit.Admin(c, tx, "moderation."+in.Action, in.TargetType, strconv.FormatInt(in.TargetID, 10),
			map[string]any{"actionId": ef.actionID})
	})
	if err != nil {
		return ModerationAction{}, asHTTP(err)
	}
	s.after(ctx, adminID, ef)
	row, err := s.repo.getAction(ctx, ef.actionID)
	if err != nil {
		return ModerationAction{}, httpx.Internal().Wrap(err)
	}
	return toAction(row), nil
}

// ListReports returns the admin queue.
func (s *Service) ListReports(c *gin.Context, f ReportFilter, p httpx.PageParams) ([]AdminReport, string, error) {
	v := map[string]string{}
	if f.Status != "" && !reportStatuses[f.Status] {
		v["status"] = "must be one of open, actioned, dismissed"
	}
	if f.TargetType != "" && !targetTypes[f.TargetType] {
		v["targetType"] = "is not a valid target type"
	}
	if f.Reason != "" && !reasons[f.Reason] {
		v["reason"] = "is not a valid reason"
	}
	if len(v) > 0 {
		return nil, "", httpx.Validation(v)
	}
	rows, err := s.repo.listReports(c.Request.Context(), f, p)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		last := rows[len(rows)-1]
		bucket := "o"
		if last.Reason == ReasonUnderageSafety {
			bucket = "u"
		}
		next = httpx.EncodeCursor(httpx.Cursor{ID: last.ID, Time: bucket})
	}
	out, err := s.buildReports(c, rows)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	return out, next, nil
}

// buildReports attaches reporter summaries and target previews. Message text is included only
// for admins with moderation.read_messages, and that read is audited (docs/security.md).
func (s *Service) buildReports(c *gin.Context, rows []reportRow) ([]AdminReport, error) {
	ctx := c.Request.Context()
	reporterIDs := make([]int64, 0, len(rows))
	byType := map[string][]int64{}
	for _, r := range rows {
		reporterIDs = append(reporterIDs, r.ReporterID)
		byType[r.TargetType] = append(byType[r.TargetType], r.TargetID)
	}
	sums, err := s.d.Users.Summaries(ctx, reporterIDs)
	if err != nil {
		return nil, err
	}
	previews := map[string]map[int64]TargetPreview{}
	for _, t := range targetTypeList {
		ids := byType[t]
		if len(ids) == 0 {
			continue
		}
		if previews[t], err = s.repo.previews(ctx, t, ids); err != nil {
			return nil, err
		}
	}
	if msgs := byType[TargetMessage]; len(msgs) > 0 {
		if authn.HasPermission(c, "moderation.read_messages") {
			if err := audit.Admin(c, s.d.DB, "moderation.read_messages", "message", "", map[string]any{"messageIds": msgs}); err != nil {
				return nil, err
			}
		} else {
			for id, p := range previews[TargetMessage] {
				p.Text = nil
				previews[TargetMessage][id] = p
			}
		}
	}
	out := make([]AdminReport, 0, len(rows))
	for _, r := range rows {
		a := AdminReport{
			ID: r.ID, TargetType: r.TargetType, TargetID: r.TargetID, TargetPreview: previews[r.TargetType][r.TargetID],
			Reason: r.Reason, Details: r.Details, Status: r.Status, OpenReportsOnTarget: r.OpenOnTarget,
			ResolutionNote: r.ResolutionNote, CreatedAt: ts(r.CreatedAt), ResolvedAt: tsPtr(r.ResolvedAt),
		}
		if sum, ok := sums[r.ReporterID]; ok {
			a.Reporter = &sum
		}
		out = append(out, a)
	}
	return out, nil
}

// ListActions returns moderation history, newest first.
func (s *Service) ListActions(ctx context.Context, targetType string, targetID int64, p httpx.PageParams) ([]ModerationAction, string, error) {
	if targetType != "" && !targetTypes[targetType] {
		return nil, "", httpx.Validation(map[string]string{"targetType": "is not a valid target type"})
	}
	if targetID > 0 && targetType == "" {
		return nil, "", httpx.Validation(map[string]string{"targetType": "is required with targetId"})
	}
	rows, err := s.repo.listActions(ctx, targetType, targetID, p)
	if err != nil {
		return nil, "", httpx.Internal().Wrap(err)
	}
	next := ""
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out := make([]ModerationAction, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAction(r))
	}
	return out, next, nil
}

func toAction(r actionRow) ModerationAction {
	a := ModerationAction{ID: r.ID, TargetType: r.TargetType, TargetID: r.TargetID, Action: r.Action,
		Reason: r.Reason, ReportID: r.ReportID, CreatedAt: ts(r.CreatedAt)}
	if r.AdminID != nil {
		ref := &AdminRef{ID: *r.AdminID}
		if r.AdminName != nil {
			ref.Name = *r.AdminName
		}
		a.Admin = ref
	}
	return a
}

func asHTTP(err error) error {
	if _, ok := httpx.As(err); ok {
		return err
	}
	return httpx.Internal().Wrap(err)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
