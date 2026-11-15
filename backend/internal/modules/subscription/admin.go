package subscription

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/userdir"
)

var planCodePattern = regexp.MustCompile(`^[a-z0-9_]{2,48}$`)

// Bounds that keep values inside the column types (not business limits).
const (
	maxPricePaise   = 4294967295 // INT UNSIGNED
	maxGrantDays    = 3660
	maxReasonLength = 500
)

// AdminPlans lists every plan, inactive ones included.
func (s *Service) AdminPlans(ctx context.Context) ([]AdminPlan, error) {
	plans, err := s.repo.listPlans(ctx, s.d.DB, "", false)
	if err != nil {
		return nil, err
	}
	if err := s.repo.withBenefits(ctx, s.d.DB, plans); err != nil {
		return nil, err
	}
	return plans, nil
}

// applyPlanInput merges in onto p; create=true requires every mandatory field.
func applyPlanInput(p *AdminPlan, in planInput, create bool) map[string]string {
	f := map[string]string{}
	missing := func(field string) {
		if create {
			f[field] = "is required"
		}
	}
	switch {
	case in.Code == nil:
		missing("code")
	case !planCodePattern.MatchString(strings.TrimSpace(*in.Code)):
		f["code"] = "must be 2-48 lowercase letters, digits or _"
	default:
		p.Code = strings.TrimSpace(*in.Code)
	}
	switch {
	case in.Audience == nil:
		missing("audience")
	case !audiences[*in.Audience]:
		f["audience"] = "must be player, coach or organiser"
	default:
		p.Audience = *in.Audience
	}
	if in.Name == nil {
		missing("name")
	} else if name := strings.TrimSpace(*in.Name); name == "" || utf8.RuneCountInString(name) > 80 {
		f["name"] = "must be 1-80 characters"
	} else {
		p.Name = name
	}
	if in.Description != nil {
		if d := strings.TrimSpace(*in.Description); utf8.RuneCountInString(d) > 500 {
			f["description"] = "must be at most 500 characters"
		} else {
			p.Description = optional(d)
		}
	}
	switch {
	case in.PricePaise == nil:
		missing("pricePaise")
	case *in.PricePaise < 0 || *in.PricePaise > maxPricePaise:
		f["pricePaise"] = "must be a non-negative amount in paise"
	default:
		p.PricePaise = *in.PricePaise
	}
	switch {
	case in.BillingPeriod == nil:
		missing("billingPeriod")
	case billingPeriods[*in.BillingPeriod] == 0:
		f["billingPeriod"] = "must be monthly, quarterly or yearly"
	default:
		p.BillingPeriod = *in.BillingPeriod
	}
	product := func(field string, src *string, dst **string) {
		if src == nil {
			return
		}
		if v := strings.TrimSpace(*src); len(v) > 128 {
			f[field] = "must be at most 128 characters"
		} else {
			*dst = optional(v)
		}
	}
	product("storeProductIdAndroid", in.StoreProductIDAndroid, &p.StoreProductIDAndroid)
	product("storeProductIdIos", in.StoreProductIDIos, &p.StoreProductIDIos)
	if in.Active != nil {
		p.Active = *in.Active
	}
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	return f
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func planIDString(id int64) string { return strconv.FormatInt(id, 10) }

// CreatePlan adds a plan (no benefits yet).
func (s *Service) CreatePlan(c *gin.Context, in planInput) (AdminPlan, error) {
	ctx := c.Request.Context()
	p := AdminPlan{Active: true, Plan: Plan{Benefits: []Benefit{}}}
	if f := applyPlanInput(&p, in, true); len(f) > 0 {
		return p, httpx.Validation(f)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if err := s.checkProducts(ctx, tx, p); err != nil {
			return err
		}
		id, err := s.repo.insertPlan(ctx, tx, p)
		if err != nil {
			return dupPlan(err)
		}
		p.ID = id
		if err := audit.Config(ctx, tx, "premium_plans", planIDString(id), nil, p, authn.AdminID(c)); err != nil {
			return err
		}
		return audit.Admin(c, tx, "plan.create", "premium_plan", planIDString(id), map[string]any{"code": p.Code})
	})
	return p, err
}

// UpdatePlan applies a partial update.
func (s *Service) UpdatePlan(c *gin.Context, id int64, in planInput) (AdminPlan, error) {
	ctx := c.Request.Context()
	var p AdminPlan
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		old, err := s.repo.getPlan(ctx, tx, id)
		if db.IsNoRows(err) {
			return errPlanNotFound
		}
		if err != nil {
			return err
		}
		p = old
		if f := applyPlanInput(&p, in, false); len(f) > 0 {
			return httpx.Validation(f)
		}
		if err := s.checkProducts(ctx, tx, p); err != nil {
			return err
		}
		if err := s.repo.updatePlan(ctx, tx, p); err != nil {
			return dupPlan(err)
		}
		if err := audit.Config(ctx, tx, "premium_plans", planIDString(id), old, p, authn.AdminID(c)); err != nil {
			return err
		}
		if err := audit.Admin(c, tx, "plan.update", "premium_plan", planIDString(id), nil); err != nil {
			return err
		}
		plans := []AdminPlan{p}
		if err := s.repo.withBenefits(ctx, tx, plans); err != nil {
			return err
		}
		p = plans[0]
		return nil
	})
	return p, err
}

func dupPlan(err error) error {
	if db.IsDuplicate(err) {
		return httpx.Conflict("A plan with this code already exists.")
	}
	return err
}

// checkProducts refuses a store product id that another plan already uses, so a store
// purchase always maps to exactly one plan.
func (s *Service) checkProducts(ctx context.Context, q db.Querier, p AdminPlan) error {
	for _, pc := range []struct {
		col string
		v   *string
	}{{"store_product_id_android", p.StoreProductIDAndroid}, {"store_product_id_ios", p.StoreProductIDIos}} {
		col, v := pc.col, pc.v
		if v == nil {
			continue
		}
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM premium_plans WHERE `+col+` = ? AND id <> ?`, *v, p.ID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return httpx.Conflict("Another plan already uses this store product id.")
		}
	}
	return nil
}

// validateBenefits checks keys against the closed list and normalises values.
func validateBenefits(in []benefitInput) ([]benefitRow, error) {
	f := map[string]string{}
	seen := map[string]bool{}
	out := make([]benefitRow, 0, len(in))
	for i, b := range in {
		field := "benefits[" + strconv.Itoa(i) + "]"
		kind, ok := benefitKinds[b.Key]
		if !ok {
			f[field+".key"] = "is not a known benefit"
			continue
		}
		if seen[b.Key] {
			f[field+".key"] = "is listed twice"
			continue
		}
		seen[b.Key] = true
		var cmp bytes.Buffer
		if json.Compact(&cmp, b.Value) != nil {
			f[field+".value"] = "is required"
			continue
		}
		v := cmp.String()
		switch kind {
		case "bool":
			if v != "true" {
				f[field+".value"] = "must be true"
			}
		case "int":
			if n, err := strconv.ParseInt(v, 10, 32); err != nil || n <= 0 {
				f[field+".value"] = "must be a positive whole number"
			}
		}
		label := strings.TrimSpace(b.Label)
		if label == "" || utf8.RuneCountInString(label) > 120 {
			f[field+".label"] = "must be 1-120 characters"
		}
		out = append(out, benefitRow{Key: b.Key, Value: json.RawMessage(v), Label: label, SortOrder: b.SortOrder})
	}
	if len(f) > 0 {
		return nil, httpx.Validation(f)
	}
	return out, nil
}

// ReplaceBenefits replaces the whole benefit list of a plan.
func (s *Service) ReplaceBenefits(c *gin.Context, id int64, in []benefitInput) (AdminPlan, error) {
	ctx := c.Request.Context()
	list, err := validateBenefits(in)
	if err != nil {
		return AdminPlan{}, err
	}
	var p AdminPlan
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if p, err = s.repo.getPlan(ctx, tx, id); db.IsNoRows(err) {
			return errPlanNotFound
		} else if err != nil {
			return err
		}
		old, err := s.repo.benefits(ctx, tx, []int64{id})
		if err != nil {
			return err
		}
		if err := s.repo.replaceBenefits(ctx, tx, id, list); err != nil {
			return err
		}
		oldList := old[id]
		if oldList == nil {
			oldList = []benefitRow{}
		}
		if err := audit.Config(ctx, tx, "premium_plan_benefits", planIDString(id), oldList, list, authn.AdminID(c)); err != nil {
			return err
		}
		return audit.Admin(c, tx, "plan.benefits_replace", "premium_plan", planIDString(id), nil)
	})
	if err != nil {
		return AdminPlan{}, err
	}
	p.Benefits = toBenefits(list)
	return p, nil
}

// AdminSubscriptions pages subscriptions by id desc.
func (s *Service) AdminSubscriptions(ctx context.Context, f adminSubFilter, page httpx.PageParams) ([]AdminSubscription, httpx.Meta, error) {
	if f.Status != "" && !statuses[f.Status] {
		return nil, httpx.Meta{}, httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	rows, err := s.repo.listSubs(ctx, s.d.DB, f, page.Cursor.ID, page.Limit)
	if err != nil {
		return nil, httpx.Meta{}, err
	}
	var meta httpx.Meta
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		meta.NextCursor = httpx.EncodeCursor(httpx.Cursor{ID: rows[len(rows)-1].ID})
	}
	out := []AdminSubscription{}
	if len(rows) == 0 {
		return out, meta, nil
	}
	subs, err := s.render(ctx, s.d.DB, rows)
	if err != nil {
		return nil, meta, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	users, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, meta, err
	}
	for i, sub := range subs {
		var u *userdir.Summary
		if v, ok := users[rows[i].UserID]; ok {
			u = &v
		}
		out = append(out, AdminSubscription{Subscription: sub, User: u})
	}
	return out, meta, nil
}

// Grant gives a user a manual subscription for req.Days days.
func (s *Service) Grant(c *gin.Context, req grantRequest) (Subscription, error) {
	ctx := c.Request.Context()
	f := map[string]string{}
	if req.UserID <= 0 {
		f["userId"] = "is required"
	}
	if req.PlanID <= 0 {
		f["planId"] = "is required"
	}
	if req.Days < 1 || req.Days > maxGrantDays {
		f["days"] = "must be between 1 and " + strconv.Itoa(maxGrantDays)
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > maxReasonLength {
		f["reason"] = "must be 1-500 characters"
	}
	if len(f) > 0 {
		return Subscription{}, httpx.Validation(f)
	}
	if _, ok, err := s.d.Users.Summary(ctx, req.UserID); err != nil {
		return Subscription{}, err
	} else if !ok {
		return Subscription{}, httpx.NotFound("User")
	}
	now := s.now()
	row := subRow{UserID: req.UserID, PlanID: req.PlanID, Store: StoreManual, Status: StatusActive,
		StartedAt: now, ExpiresAt: now.AddDate(0, 0, req.Days)}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		if _, err := s.repo.getPlan(ctx, tx, req.PlanID); db.IsNoRows(err) {
			return errPlanNotFound
		} else if err != nil {
			return err
		}
		id, err := s.repo.insertSub(ctx, tx, row, nil)
		if err != nil {
			return err
		}
		row.ID = id
		return audit.Admin(c, tx, "subscription.grant", "subscription", strconv.FormatInt(id, 10),
			map[string]any{"userId": req.UserID, "planId": req.PlanID, "days": req.Days, "reason": req.Reason})
	})
	if err != nil {
		return Subscription{}, err
	}
	s.publish(ctx, []events.Event{subEvent(events.SubscriptionActivated, row)})
	return s.renderOne(ctx, row)
}
