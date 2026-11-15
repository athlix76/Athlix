package listingpayment

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/modules/event"
	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/httpx"
)

// AdminOrders is GET /admin/listing-orders (payments.read).
func (s *Service) AdminOrders(c *gin.Context, f OrderFilter, pg httpx.PageParams) ([]AdminOrder, string, error) {
	ctx := c.Request.Context()
	if f.Status != "" && !orderStatuses[f.Status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid order status"})
	}
	rs, err := s.r.adminOrders(ctx, f, pg)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rs[len(rs)-1].ID})
	}
	views, err := s.views(ctx, rs)
	if err != nil {
		return nil, "", err
	}
	ids, seen := []int64{}, map[int64]bool{}
	for _, o := range rs {
		if !seen[o.OrganiserID] {
			seen[o.OrganiserID] = true
			ids = append(ids, o.OrganiserID)
		}
	}
	users, err := s.d.Users.Summaries(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]AdminOrder, 0, len(rs))
	for i, v := range views {
		a := AdminOrder{Order: v}
		if u, ok := users[rs[i].OrganiserID]; ok {
			u := u
			a.Organiser = &u
		}
		out = append(out, a)
	}
	return out, next, nil
}

// AdminRefund is POST /admin/listing-orders/:id/refund (payments.refund).
func (s *Service) AdminRefund(c *gin.Context, id int64, in RefundInput) (Order, error) {
	ctx := c.Request.Context()
	reason := strings.TrimSpace(in.Reason)
	fields := map[string]string{}
	if in.AmountPaise <= 0 {
		fields["amountPaise"] = "must be a positive amount in paise"
	}
	if reason == "" || len([]rune(reason)) > maxReasonLen {
		fields["reason"] = "is required (max 255 characters)"
	}
	if len(fields) > 0 {
		return Order{}, httpx.Validation(fields)
	}
	now := s.now()
	ob := &outbox{}
	var out orderRec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		pre, err := s.r.orderByID(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if _, err := s.r.event(ctx, tx, pre.EventID, 0, true); err != nil {
			return err
		}
		o, err := s.r.orderByID(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if o.Status != stPaid && o.Status != stPartially {
			return httpx.InvalidState("Only paid orders can be refunded.")
		}
		if remaining := o.TotalPaise - o.RefundPaise; in.AmountPaise > remaining {
			return httpx.Validation(map[string]string{"amountPaise": "must be at most " + strconv.FormatInt(remaining, 10)})
		}
		if err := s.refund(ctx, tx, &o, in.AmountPaise, reason, now, ob); err != nil {
			return err
		}
		out = o
		return audit.Admin(c, tx, "listing.refund", "listing_order", strconv.FormatInt(id, 10),
			map[string]any{"amountPaise": in.AmountPaise, "eventId": o.EventID, "status": o.Status})
	})
	if err != nil {
		return Order{}, err
	}
	s.flush(ctx, authn.AdminID(c), ob)
	return s.view(ctx, out)
}

// ---------- fee rules ----------

// FeeRules is GET /admin/fee-rules (config.read).
func (s *Service) FeeRules(c *gin.Context) ([]FeeRule, error) {
	return s.r.allRules(c.Request.Context())
}

// Preview is POST /admin/fee-rules/preview (config.read): the quote the current rules give.
func (s *Service) Preview(c *gin.Context, in QuoteInput) (Quote, error) {
	if err := s.validateQuote(c.Request.Context(), in, false); err != nil {
		return Quote{}, err
	}
	return s.quoteFor(c.Request.Context(), in.SportID, in.EventType, in.EntryFeePaise, in.MaxSlots, s.now())
}

// merge applies the input onto r. create requires every non-nullable field.
func merge(r *FeeRule, in FeeRuleInput, create bool, fields map[string]string) {
	req := func(name string, set bool) {
		if create && !set {
			fields[name] = "is required"
		}
	}
	if in.Name.Set {
		r.Name = strings.TrimSpace(in.Name.V)
	}
	if in.Name.Set || create {
		if n := len([]rune(r.Name)); n == 0 || n > maxRuleNameLen {
			fields["name"] = "is required (max 80 characters)"
		}
	}
	if in.SportID.Set {
		r.SportID = nil
		if in.SportID.Valid {
			if in.SportID.V <= 0 {
				fields["sportId"] = "must be a sport id or null"
			}
			v := in.SportID.V
			r.SportID = &v
		}
	}
	if in.EventType.Set {
		r.EventType = nil
		if in.EventType.Valid {
			if !eventTypes[in.EventType.V] {
				fields["eventType"] = "must be tournament, trial, camp, match, other or null"
			}
			v := in.EventType.V
			r.EventType = &v
		}
	}
	bounded := func(name string, o event.Opt[int64], dst *int64, lo, hi int64) {
		req(name, o.Set && o.Valid)
		if !o.Set {
			return
		}
		if !o.Valid || o.V < lo || o.V > hi {
			fields[name] = "must be between " + strconv.FormatInt(lo, 10) + " and " + strconv.FormatInt(hi, 10)
			return
		}
		*dst = o.V
	}
	bounded("percentBp", in.PercentBp, &r.PercentBp, 0, maxBp)
	bounded("minFeePaise", in.MinFeePaise, &r.MinFeePaise, 0, maxUint32)
	bounded("freeEventFeePaise", in.FreeEventFeePaise, &r.FreeEventFeePaise, 0, maxUint32)
	bounded("gstBp", in.GstBp, &r.GstBp, 0, maxBp)
	if in.Priority.Set {
		if !in.Priority.Valid || in.Priority.V < -1<<31 || in.Priority.V > 1<<31-1 {
			fields["priority"] = "must be a whole number"
		} else {
			r.Priority = in.Priority.V
		}
	}
	if in.MaxFeePaise.Set {
		r.MaxFeePaise = nil
		if in.MaxFeePaise.Valid {
			v := in.MaxFeePaise.V
			r.MaxFeePaise = &v
		}
	}
	if r.MaxFeePaise != nil && (*r.MaxFeePaise < r.MinFeePaise || *r.MaxFeePaise > maxUint32) {
		fields["maxFeePaise"] = "must be null or between minFeePaise and " + strconv.FormatInt(maxUint32, 10)
	}
	if in.Active.Set {
		if !in.Active.Valid {
			fields["active"] = "must be true or false"
		} else {
			r.Active = in.Active.V
		}
	}
	if in.EffectiveFrom.Set {
		if !in.EffectiveFrom.Valid {
			fields["effectiveFrom"] = "must be a timestamp"
		} else {
			r.EffectiveFrom = in.EffectiveFrom.V.UTC()
		}
	}
	if in.EffectiveTo.Set {
		r.EffectiveTo = nil
		if in.EffectiveTo.Valid {
			v := in.EffectiveTo.V.UTC()
			r.EffectiveTo = &v
		}
	}
	if r.EffectiveTo != nil && !r.EffectiveTo.After(r.EffectiveFrom) {
		fields["effectiveTo"] = "must be after effectiveFrom"
	}
}

// CreateFeeRule is POST /admin/fee-rules (fees.write).
func (s *Service) CreateFeeRule(c *gin.Context, in FeeRuleInput) (FeeRule, error) {
	ctx := c.Request.Context()
	r := FeeRule{GstBp: 1800, Active: true, EffectiveFrom: s.now().Truncate(time.Millisecond)}
	if !in.GstBp.Set {
		in.GstBp.Set, in.GstBp.Valid, in.GstBp.V = true, true, r.GstBp // column default 1800
	}
	fields := map[string]string{}
	merge(&r, in, true, fields)
	if len(fields) > 0 {
		return FeeRule{}, httpx.Validation(fields)
	}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		id, err := s.r.insertRule(ctx, tx, r, authn.AdminID(c))
		if err != nil {
			return err
		}
		r.ID = id
		rid := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "listing_fee_rules", rid, nil, r, authn.AdminID(c)); err != nil {
			return err
		}
		return audit.Admin(c, tx, "fee_rule.create", "listing_fee_rule", rid, map[string]any{"active": r.Active})
	})
	return r, err
}

// UpdateFeeRule is PUT /admin/fee-rules/:id (fees.write). Only future quotes change: orders
// keep their snapshot. The last active default rule cannot be deactivated or narrowed.
func (s *Service) UpdateFeeRule(c *gin.Context, id int64, in FeeRuleInput) (FeeRule, error) {
	ctx := c.Request.Context()
	var out FeeRule
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		old, err := s.r.rule(ctx, tx, id)
		if err != nil {
			return err
		}
		r := old
		fields := map[string]string{}
		merge(&r, in, false, fields)
		if len(fields) > 0 {
			return httpx.Validation(fields)
		}
		if old.Active && old.isDefault() && !(r.Active && r.isDefault()) {
			n, err := s.r.otherActiveDefaults(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return httpx.InvalidState("At least one active default fee rule (no sport, no event type) must remain.")
			}
		}
		if err := s.r.updateRule(ctx, tx, r); err != nil {
			return err
		}
		rid := strconv.FormatInt(id, 10)
		if err := audit.Config(ctx, tx, "listing_fee_rules", rid, old, r, authn.AdminID(c)); err != nil {
			return err
		}
		out = r
		return audit.Admin(c, tx, "fee_rule.update", "listing_fee_rule", rid, map[string]any{"active": r.Active})
	})
	return out, err
}
