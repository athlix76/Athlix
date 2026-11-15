package listingpayment

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/razorpay"
)

// Service holds the listing-fee rules.
type Service struct {
	d *app.Deps
	r *repo
	// rz builds the Razorpay client; tests replace it with a razorpay.Fake.
	rz func(ctx context.Context) (razorpay.Client, error)
}

func newService(d *app.Deps) *Service {
	s := &Service{d: d, r: &repo{db: d.DB}}
	s.rz = func(ctx context.Context) (razorpay.Client, error) {
		c, err := razorpay.FromSecrets(ctx, d.Secrets, d.Cfg.SecretName(app.SecretRazorpay))
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	return s
}

func (s *Service) now() time.Time { return s.d.Now().UTC() }

func (s *Service) log(ctx context.Context) *slog.Logger { return logger.From(ctx) }

// ---------- quote ----------

func (s *Service) refundPolicy(ctx context.Context) RefundPolicy {
	st := s.d.Settings
	return RefundPolicy{
		BeforeFirstRegistrationPercent: st.Int(ctx, "listing.refund.before_first_registration_percent", defRefundBefore),
		AfterRegistrationsPercent:      st.Int(ctx, "listing.refund.after_registrations_percent", defRefundAfter),
		OnAdminRejectionPercent:        st.Int(ctx, "listing.refund.on_admin_rejection_percent", defRefundRejection),
	}
}

// policy fills the refund policy shown before payment (current settings + active document).
func (s *Service) policy(ctx context.Context, q *Quote) error {
	q.RefundPolicy = s.refundPolicy(ctx)
	id, err := s.r.policyDocID(ctx)
	q.RefundPolicyDocumentID = id
	return err
}

// quoteFor selects the rule and computes the fee.
func (s *Service) quoteFor(ctx context.Context, sportID int64, eventType string, fee, slots int64, now time.Time) (Quote, error) {
	rules, err := s.r.candidateRules(ctx, sportID, eventType, now)
	if err != nil {
		return Quote{}, err
	}
	rule, ok := pick(rules)
	if !ok {
		s.log(ctx).Error("no active listing fee rule", slog.Int64("sportId", sportID), slog.String("eventType", eventType))
		return Quote{}, httpx.InvalidState("Listing fees are not configured right now.")
	}
	q := compute(rule, fee, slots)
	return q, s.policy(ctx, &q)
}

func (s *Service) validateQuote(ctx context.Context, in QuoteInput, checkFree bool) error {
	fields := map[string]string{}
	if in.SportID <= 0 {
		fields["sportId"] = "is required"
	}
	if !eventTypes[in.EventType] {
		fields["eventType"] = "must be tournament, trial, camp, match or other"
	}
	if limit := int64(s.d.Settings.Int(ctx, "event.max_slots_limit", defMaxSlotsLimit)); in.MaxSlots < 1 || in.MaxSlots > limit {
		fields["maxSlots"] = "must be between 1 and " + strconv.FormatInt(limit, 10)
	}
	switch {
	case in.EntryFeePaise < 0 || in.EntryFeePaise > maxUint32:
		fields["entryFeePaise"] = "is out of range"
	case checkFree && in.EntryFeePaise == 0 && !s.d.Settings.Bool(ctx, "event.allow_free_events", defAllowFree):
		fields["entryFeePaise"] = "free events are not allowed"
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return nil
}

// Quote is GET /listing/quote (calculator before creating an event).
func (s *Service) Quote(ctx context.Context, in QuoteInput) (Quote, error) {
	if err := s.validateQuote(ctx, in, true); err != nil {
		return Quote{}, err
	}
	return s.quoteFor(ctx, in.SportID, in.EventType, in.EntryFeePaise, in.MaxSlots, s.now())
}

// EventQuote is POST /events/:id/listing/quote (owner).
func (s *Service) EventQuote(ctx context.Context, uid, eventID int64) (Quote, error) {
	e, err := s.r.event(ctx, nil, eventID, uid, false)
	if err != nil {
		return Quote{}, err
	}
	return s.quoteFor(ctx, e.SportID, e.EventType, e.EntryFeePaise, e.MaxSlots, s.now())
}

// submitCheck is the "submitting for listing" validation (docs/api.md, Events). It is
// repeated here because the start-time rule depends on when the order is made.
func (s *Service) submitCheck(ctx context.Context, e evRow, now time.Time) error {
	fields := map[string]string{}
	h := s.d.Settings.Int(ctx, "event.min_hours_before_start", defMinHoursBefore)
	if e.StartsAt.Before(now.Add(time.Duration(h) * time.Hour)) {
		fields["startsAt"] = "must be at least " + strconv.Itoa(h) + " hours from now"
	}
	if e.Deadline.After(e.StartsAt) {
		fields["registrationDeadline"] = "must not be after startsAt"
	}
	if e.EndsAt.Before(e.StartsAt) {
		fields["endsAt"] = "must not be before startsAt"
	}
	if limit := int64(s.d.Settings.Int(ctx, "event.max_slots_limit", defMaxSlotsLimit)); e.MaxSlots < 1 || e.MaxSlots > limit {
		fields["maxSlots"] = "must be between 1 and " + strconv.FormatInt(limit, 10)
	}
	if e.EntryFeePaise == 0 && !s.d.Settings.Bool(ctx, "event.allow_free_events", defAllowFree) {
		fields["entryFeePaise"] = "free events are not allowed"
	}
	if e.ParticipantType == "team" {
		if e.TeamSizeMin == nil || e.TeamSizeMax == nil || *e.TeamSizeMin < 1 || *e.TeamSizeMin > *e.TeamSizeMax || *e.TeamSizeMax > maxTeamMembers {
			fields["teamSizeMin"] = "must satisfy 1 ≤ teamSizeMin ≤ teamSizeMax ≤ " + strconv.Itoa(maxTeamMembers)
		}
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return nil
}

// ---------- provider ----------

// provider is listing.payment_provider; mock is refused in production.
func (s *Service) provider(ctx context.Context) (string, error) {
	p := s.d.Settings.String(ctx, "listing.payment_provider", defProvider)
	switch {
	case p == providerRazorpay:
		return p, nil
	case p == providerMock && !s.d.Cfg.IsProd():
		return p, nil
	}
	s.log(ctx).Error("listing payment provider unavailable", slog.String("provider", p))
	return "", httpx.Upstream("Payments are not available right now. Try again later.")
}

// mockAllowed reports whether POST /listing/mock/pay is enabled.
func (s *Service) mockAllowed(ctx context.Context) bool {
	return !s.d.Cfg.IsProd() && s.d.Settings.String(ctx, "listing.payment_provider", defProvider) == providerMock
}

func (s *Service) client(ctx context.Context) (razorpay.Client, error) {
	c, err := s.rz(ctx)
	if err != nil {
		s.log(ctx).Error("razorpay client unavailable", slog.String("err", err.Error()))
		return nil, httpx.Upstream("Payments are not available right now. Try again later.")
	}
	return c, nil
}

func (s *Service) providerOrder(ctx context.Context, tx *sql.Tx, o *orderRec) error {
	c, err := s.client(ctx)
	if err != nil {
		return err
	}
	id := strconv.FormatInt(o.ID, 10)
	ro, err := c.CreateOrder(ctx, o.TotalPaise, "listing_"+id, map[string]string{"listingOrderId": id, "eventId": strconv.FormatInt(o.EventID, 10)})
	if err != nil {
		s.log(ctx).Error("razorpay create order", slog.Int64("orderId", o.ID), slog.String("err", err.Error()))
		return httpx.Upstream("The payment provider did not respond. Try again.")
	}
	if err := s.r.setProviderOrder(ctx, tx, o.ID, ro.ID); err != nil {
		return err
	}
	o.ProviderOrderID = &ro.ID
	return nil
}

// ---------- order ----------

var orderable = []string{"draft", "rejected", "pending_payment"}

// CreateOrder is POST /events/:id/listing/order (owner).
func (s *Service) CreateOrder(ctx context.Context, uid, eventID int64) (Order, error) {
	now := s.now()
	ob := &outbox{}
	var out orderRec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		e, err := s.r.event(ctx, tx, eventID, uid, true)
		if err != nil {
			return err
		}
		switch e.Status {
		case "draft", "rejected", "pending_payment":
		default:
			return httpx.InvalidState("This event is " + e.Status + " and cannot be submitted for listing.")
		}
		if err := s.submitCheck(ctx, e, now); err != nil {
			return err
		}
		q, err := s.quoteFor(ctx, e.SportID, e.EventType, e.EntryFeePaise, e.MaxSlots, now)
		if err != nil {
			return err
		}
		opens, err := s.r.eventOrders(ctx, tx, eventID, stCreated, stFailed)
		if err != nil {
			return err
		}
		var reuse *orderRec
		var stale []int64
		for i := range opens {
			o := opens[i]
			if reuse == nil && o.Status == stCreated && o.sameQuote(q) && o.ExpiresAt.After(now) {
				reuse = &opens[i]
				continue
			}
			stale = append(stale, o.ID)
		}
		if err := s.r.expire(ctx, tx, stale); err != nil {
			return err
		}
		if reuse != nil {
			if reuse.Provider == providerRazorpay && reuse.ProviderOrderID == nil {
				if err := s.providerOrder(ctx, tx, reuse); err != nil {
					return err
				}
			}
			if _, err := s.r.setEventStatus(ctx, tx, eventID, orderable, "pending_payment", now); err != nil {
				return err
			}
			out = *reuse
			out.EventStatus = "pending_payment"
			return nil
		}
		o := orderRec{EventID: eventID, OrganiserID: uid, FeeRuleID: q.FeeRuleID, EntryFeePaise: q.EntryFeePaise,
			MaxSlots: q.MaxSlots, PercentBp: q.PercentBp, GstBp: q.GstBp, BaseFeePaise: q.BaseFeePaise,
			GstPaise: q.GstPaise, TotalPaise: q.TotalPaise, Status: stCreated, RuleName: q.RuleName,
			MinFeePaise: q.MinFeePaise, MaxFeePaise: q.MaxFeePaise, CreatedAt: now,
			ExpiresAt: now.Add(time.Duration(s.d.Settings.Int(ctx, "listing.order_ttl_minutes", defOrderTTLMinutes)) * time.Minute)}
		if q.TotalPaise == 0 {
			o.Provider, o.Status, o.PaidAt = providerNone, stPaid, &now
		} else if o.Provider, err = s.provider(ctx); err != nil {
			return err
		}
		if o.ID, err = s.r.insertOrder(ctx, tx, o); err != nil {
			return err
		}
		if o.Provider == providerRazorpay {
			if err := s.providerOrder(ctx, tx, &o); err != nil {
				return err
			}
		}
		if o.Status == stPaid {
			next, err := s.advance(ctx, tx, e, orderable, now, ob)
			if err != nil {
				return err
			}
			o.EventStatus = next
			ob.paid = append(ob.paid, o)
		} else {
			if _, err := s.r.setEventStatus(ctx, tx, eventID, orderable, "pending_payment", now); err != nil {
				return err
			}
			o.EventStatus = "pending_payment"
		}
		out = o
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	s.flush(ctx, uid, ob)
	return s.view(ctx, out)
}

// advance moves a paid event to pending_review or published.
func (s *Service) advance(ctx context.Context, tx *sql.Tx, e evRow, from []string, now time.Time, ob *outbox) (string, error) {
	next, typ := "published", events.EventPublished
	if s.d.Settings.Bool(ctx, "event.require_admin_approval", defRequireApproval) {
		next, typ = "pending_review", events.EventSubmitted
	}
	ok, err := s.r.setEventStatus(ctx, tx, e.ID, from, next, now)
	if err != nil {
		return "", err
	}
	if !ok {
		return e.Status, nil
	}
	e.Status = next
	ob.statuses = append(ob.statuses, statusChange{typ: typ, ev: e})
	return next, nil
}

// settle records a payment on o (e and o are locked). A repeat is a no-op. A payment that
// arrives after the order stopped being payable (expired, or the event left pending_payment)
// is recorded and refunded in full; late is true then.
func (s *Service) settle(ctx context.Context, tx *sql.Tx, e evRow, o *orderRec, paymentID string, now time.Time, ob *outbox) (late bool, err error) {
	if o.settled() {
		return false, nil
	}
	pid := &paymentID
	if err := s.r.markPaid(ctx, tx, o.ID, pid, now); err != nil {
		return false, err
	}
	payable := o.open() && e.Status == "pending_payment"
	o.Status, o.PaidAt, o.ProviderPaymentID = stPaid, &now, pid
	if !payable {
		s.log(ctx).Warn("late listing payment; refunding", slog.Int64("orderId", o.ID), slog.Int64("eventId", e.ID))
		return true, s.refund(ctx, tx, o, o.TotalPaise, "late_payment", now, ob)
	}
	next, err := s.advance(ctx, tx, e, []string{"pending_payment"}, now, ob)
	if err != nil {
		return false, err
	}
	o.EventStatus = next
	ob.paid = append(ob.paid, *o)
	return false, nil
}

// refund refunds amount of a paid order (row locked) through its provider.
func (s *Service) refund(ctx context.Context, tx *sql.Tx, o *orderRec, amount int64, reason string, now time.Time, ob *outbox) error {
	if remaining := o.TotalPaise - o.RefundPaise; amount > remaining {
		amount = remaining
	}
	if amount <= 0 {
		return nil
	}
	var refundID *string
	var at *time.Time
	if o.Provider == providerRazorpay {
		if o.ProviderPaymentID == nil {
			return httpx.InvalidState("This order has no captured payment to refund.")
		}
		c, err := s.client(ctx)
		if err != nil {
			return err
		}
		rf, err := c.CreateRefund(ctx, *o.ProviderPaymentID, amount, "lr_"+strconv.FormatInt(o.ID, 10)+"_"+strconv.FormatInt(o.RefundPaise+amount, 10))
		if err != nil {
			s.log(ctx).Error("razorpay refund", slog.Int64("orderId", o.ID), slog.String("err", err.Error()))
			return httpx.Upstream("The payment provider did not accept the refund. Try again.")
		}
		refundID = &rf.ID
		if rf.Status == "processed" {
			at = &now
		}
	} else {
		at = &now
	}
	status := stPartially
	if o.RefundPaise+amount >= o.TotalPaise {
		status = stRefunded
	}
	if err := s.r.addRefund(ctx, tx, *o, amount, status, refundID, reason, at); err != nil {
		return err
	}
	o.RefundPaise += amount
	o.Status = status
	ob.refunded = append(ob.refunded, refunded{o: *o, amount: amount})
	return nil
}

// Confirm is POST /events/:id/listing/confirm (owner, Razorpay Checkout result).
func (s *Service) Confirm(ctx context.Context, uid, eventID int64, in ConfirmInput) (Order, error) {
	fields := map[string]string{}
	if in.OrderID <= 0 {
		fields["orderId"] = "is required"
	}
	if !razorpay.ValidPaymentID(in.ProviderPaymentID) {
		fields["providerPaymentId"] = "is not a valid payment id"
	}
	if in.ProviderSignature == "" {
		fields["providerSignature"] = "is required"
	}
	if len(fields) > 0 {
		return Order{}, httpx.Validation(fields)
	}
	now := s.now()
	ob := &outbox{}
	var out orderRec
	var late bool
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		e, err := s.r.event(ctx, tx, eventID, uid, true)
		if err != nil {
			return err
		}
		o, err := s.r.ownedOrder(ctx, tx, in.OrderID, eventID, uid, true)
		if err != nil {
			return err
		}
		if o.Provider != providerRazorpay || o.ProviderOrderID == nil {
			return httpx.InvalidState("This order is not paid through Razorpay.")
		}
		if o.ProviderPaymentID != nil && *o.ProviderPaymentID == in.ProviderPaymentID && o.settled() {
			out = o // already confirmed (or via the webhook)
			return nil
		}
		c, err := s.client(ctx)
		if err != nil {
			return err
		}
		if !c.VerifyPayment(*o.ProviderOrderID, in.ProviderPaymentID, in.ProviderSignature) {
			s.log(ctx).Warn("listing confirm signature mismatch", slog.Int64("orderId", o.ID), slog.Int64("userId", uid))
			return httpx.Validation(map[string]string{"providerSignature": "does not match this order"})
		}
		late, err = s.settle(ctx, tx, e, &o, in.ProviderPaymentID, now, ob)
		out = o
		return err
	})
	if err != nil {
		return Order{}, err
	}
	s.flush(ctx, uid, ob)
	if late {
		return Order{}, httpx.InvalidState("This order was no longer open; the payment is being refunded.")
	}
	return s.view(ctx, out)
}

// MockPay is POST /listing/mock/pay (owner; mock provider outside production only).
func (s *Service) MockPay(ctx context.Context, uid int64, in MockPayInput) (Order, error) {
	if !s.mockAllowed(ctx) {
		return Order{}, httpx.NotFound("Resource")
	}
	if in.OrderID <= 0 {
		return Order{}, httpx.Validation(map[string]string{"orderId": "is required"})
	}
	now := s.now()
	ob := &outbox{}
	var out orderRec
	var late bool
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		pre, err := s.r.ownedOrder(ctx, tx, in.OrderID, 0, uid, false)
		if err != nil {
			return err
		}
		e, err := s.r.event(ctx, tx, pre.EventID, uid, true)
		if err != nil {
			return err
		}
		o, err := s.r.orderByID(ctx, tx, pre.ID, true)
		if err != nil {
			return err
		}
		if o.Provider != providerMock {
			return httpx.InvalidState("This order is not a mock order.")
		}
		late, err = s.settle(ctx, tx, e, &o, "mock_pay_"+strconv.FormatInt(o.ID, 10), now, ob)
		out = o
		return err
	})
	if err != nil {
		return Order{}, err
	}
	s.flush(ctx, uid, ob)
	if late {
		return Order{}, httpx.InvalidState("This order was no longer open; the payment is being refunded.")
	}
	return s.view(ctx, out)
}

// ---------- webhook ----------

// Webhook handles POST /webhooks/razorpay. The signature is checked before anything else;
// the payload is never stored or logged (only ids and the type).
func (s *Service) Webhook(ctx context.Context, body []byte, signature, eventIDHeader string) error {
	c, err := s.client(ctx)
	if err != nil {
		return err
	}
	if signature == "" || !c.VerifyWebhook(body, signature) {
		s.log(ctx).Warn("razorpay webhook signature mismatch")
		return httpx.Unauthenticated()
	}
	ev, err := razorpay.ParseWebhook(body)
	if err != nil {
		return httpx.BadRequest("Unreadable webhook body.")
	}
	evID := eventIDHeader
	if evID == "" || len(evID) > 128 {
		sum := sha256.Sum256(body)
		evID = hex.EncodeToString(sum[:])
	}
	var providerOrderID, paymentID, entity string
	var amount int64
	switch ev.Event {
	case "payment.captured", "payment.failed":
		if ev.Payment != nil {
			providerOrderID, paymentID, entity, amount = ev.Payment.OrderID, ev.Payment.ID, ev.Payment.ID, ev.Payment.Amount
		}
	case "order.paid":
		if ev.Order != nil {
			providerOrderID, entity = ev.Order.ID, ev.Order.ID
		}
		if ev.Payment != nil {
			paymentID, amount = ev.Payment.ID, ev.Payment.Amount
		}
	case "refund.processed":
		if ev.Refund != nil {
			entity = ev.Refund.ID
		}
	}
	now := s.now()
	ob := &outbox{}
	err = db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var ptr *string
		if entity != "" {
			ptr = &entity
		}
		if ev.Event == "refund.processed" && entity != "" {
			fresh, err := s.r.recordWebhook(ctx, tx, evID, ev.Event, ptr, "processed", now)
			if err != nil || !fresh {
				return err
			}
			_, err = s.r.refundProcessed(ctx, tx, entity, now)
			return err
		}
		var orderID, eventID int64
		if providerOrderID != "" {
			if orderID, eventID, err = s.r.orderIDByProvider(ctx, tx, providerOrderID); err != nil {
				return err
			}
		}
		handled := orderID > 0 && (ev.Event == "payment.failed" || paymentID != "")
		if !handled {
			_, err := s.r.recordWebhook(ctx, tx, evID, ev.Event, ptr, "ignored", now)
			return err
		}
		fresh, err := s.r.recordWebhook(ctx, tx, evID, ev.Event, ptr, "processed", now)
		if err != nil || !fresh {
			return err
		}
		e, err := s.r.event(ctx, tx, eventID, 0, true)
		if err != nil {
			return err
		}
		o, err := s.r.orderByID(ctx, tx, orderID, true)
		if err != nil {
			return err
		}
		if ev.Event == "payment.failed" {
			return s.r.markFailed(ctx, tx, o.ID)
		}
		if amount != o.TotalPaise {
			s.log(ctx).Error("razorpay amount mismatch", slog.Int64("orderId", o.ID))
			return nil
		}
		_, err = s.settle(ctx, tx, e, &o, paymentID, now, ob)
		return err
	})
	if err != nil {
		var he *httpx.Error
		if !errors.As(err, &he) {
			s.log(ctx).Error("razorpay webhook", slog.String("type", ev.Event), slog.String("err", err.Error()))
		}
		return err
	}
	s.flush(ctx, 0, ob)
	return nil
}

// ---------- event handlers ----------

// OnCancelled handles event.cancelled: expires open orders and refunds a paid order per the
// event's final status (cancelled → before/after-registrations %, removed → none).
func (s *Service) OnCancelled(ctx context.Context, e events.Event) error {
	return s.onEventEnded(ctx, e, func(ctx context.Context, ev evRow) (int, string, bool) {
		switch ev.Status {
		case "cancelled":
			p := s.refundPolicy(ctx)
			if ev.RegistrationsCount == 0 {
				return p.BeforeFirstRegistrationPercent, "event_cancelled", true
			}
			return p.AfterRegistrationsPercent, "event_cancelled", true
		case "removed":
			return 0, "", true
		}
		return 0, "", false
	})
}

// OnRejected handles event.rejected: refunds listing.refund.on_admin_rejection_percent.
func (s *Service) OnRejected(ctx context.Context, e events.Event) error {
	return s.onEventEnded(ctx, e, func(ctx context.Context, ev evRow) (int, string, bool) {
		if ev.Status != "rejected" {
			return 0, "", false
		}
		return s.refundPolicy(ctx).OnAdminRejectionPercent, "admin_rejected", true
	})
}

// onEventEnded expires the event's open orders and refunds its paid orders by percent.
// Only orders still in `paid` are refunded, so re-delivery refunds nothing twice.
func (s *Service) onEventEnded(ctx context.Context, e events.Event, rule func(context.Context, evRow) (int, string, bool)) error {
	var p events.EventPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 {
		s.log(ctx).Warn("bad event payload", slog.String("type", e.Type))
		return nil
	}
	now := s.now()
	ob := &outbox{}
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		ev, err := s.r.event(ctx, tx, p.EventID, 0, true)
		if he, ok := httpx.As(err); ok && he.Code == httpx.CodeNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		pct, reason, ok := rule(ctx, ev)
		if !ok {
			return nil // the event moved on (e.g. resubmitted); nothing to do
		}
		if err := s.expireOpen(ctx, tx, ev.ID); err != nil {
			return err
		}
		if pct <= 0 {
			return nil
		}
		paid, err := s.r.eventOrders(ctx, tx, ev.ID, stPaid)
		if err != nil {
			return err
		}
		for i := range paid {
			o := paid[i]
			if err := s.refund(ctx, tx, &o, percentOf(o.TotalPaise, pct), reason, now, ob); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.flush(ctx, 0, ob)
	return nil
}

func (s *Service) expireOpen(ctx context.Context, tx *sql.Tx, eventID int64) error {
	opens, err := s.r.eventOrders(ctx, tx, eventID, stCreated, stFailed)
	if err != nil || len(opens) == 0 {
		return err
	}
	ids := make([]int64, 0, len(opens))
	for _, o := range opens {
		ids = append(ids, o.ID)
	}
	return s.r.expire(ctx, tx, ids)
}

// OnUpdated handles event.updated: an event that left pending_payment (edited back to
// draft) can no longer be paid with its old order, so open orders expire.
func (s *Service) OnUpdated(ctx context.Context, e events.Event) error {
	var p events.EventPayload
	if err := e.Decode(&p); err != nil || p.EventID <= 0 {
		s.log(ctx).Warn("bad event payload", slog.String("type", e.Type))
		return nil
	}
	return db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		ev, err := s.r.event(ctx, tx, p.EventID, 0, true)
		if he, ok := httpx.As(err); ok && he.Code == httpx.CodeNotFound {
			return nil
		}
		if err != nil || ev.Status == "pending_payment" {
			return err
		}
		return s.expireOpen(ctx, tx, ev.ID)
	})
}

// ---------- job ----------

// ExpireOrders is the listing.expire_orders job: open orders past expires_at → expired and
// their events back to draft. Batches of ≤ 500; safe to re-run.
func (s *Service) ExpireOrders(ctx context.Context) error {
	now := s.now()
	for {
		var n int
		err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
			ids, evs, err := s.r.dueOrders(ctx, tx, now, batchSize)
			if err != nil {
				return err
			}
			n = len(ids)
			if err := s.r.expire(ctx, tx, ids); err != nil {
				return err
			}
			seen := map[int64]bool{}
			for _, ev := range evs {
				if seen[ev] {
					continue
				}
				seen[ev] = true
				if _, err := s.r.revertEvent(ctx, tx, ev, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if n < batchSize {
			return nil
		}
	}
}

// ---------- events out / views ----------

func (s *Service) publish(ctx context.Context, typ string, actor int64, payload any, id int64) {
	if err := s.d.Events.Publish(ctx, events.New(typ, actor, payload)); err != nil {
		s.log(ctx).Error("publish event", slog.String("type", typ), slog.Int64("id", id), slog.String("err", err.Error()))
	}
}

// flush publishes after commit.
func (s *Service) flush(ctx context.Context, actor int64, ob *outbox) {
	for _, o := range ob.paid {
		s.publish(ctx, events.ListingPaid, actor, events.ListingPayload{OrderID: o.ID, EventID: o.EventID,
			OrganiserID: o.OrganiserID, AmountPaise: o.TotalPaise}, o.ID)
	}
	for _, c := range ob.statuses {
		s.publish(ctx, c.typ, actor, events.EventPayload{EventID: c.ev.ID, OrganiserID: c.ev.OrganiserID, SportID: c.ev.SportID}, c.ev.ID)
	}
	for _, r := range ob.refunded {
		s.publish(ctx, events.ListingRefunded, actor, events.ListingPayload{OrderID: r.o.ID, EventID: r.o.EventID,
			OrganiserID: r.o.OrganiserID, AmountPaise: r.amount}, r.o.ID)
	}
}

// LatestOrder returns the organiser's newest listing order for the event (404 when none or not owned).
func (s *Service) LatestOrder(ctx context.Context, uid, eventID int64) (Order, error) {
	o, err := s.r.latestOwnedOrder(ctx, eventID, uid)
	if err != nil {
		return Order{}, err
	}
	return s.view(ctx, o)
}

func (s *Service) view(ctx context.Context, o orderRec) (Order, error) {
	vs, err := s.views(ctx, []orderRec{o})
	if err != nil {
		return Order{}, err
	}
	return vs[0], nil
}

// views renders orders. providerKeyId is the public Razorpay key id (never the secret).
func (s *Service) views(ctx context.Context, os []orderRec) ([]Order, error) {
	out := make([]Order, 0, len(os))
	if len(os) == 0 {
		return out, nil
	}
	policy := s.refundPolicy(ctx)
	doc, err := s.r.policyDocID(ctx)
	if err != nil {
		return nil, err
	}
	var keyID *string
	for _, o := range os {
		if o.Provider == providerRazorpay && keyID == nil {
			if c, err := s.rz(ctx); err == nil {
				k := c.KeyID()
				keyID = &k
			}
		}
	}
	for _, o := range os {
		q := Quote{FeeRuleID: o.FeeRuleID, RuleName: o.RuleName, EntryFeePaise: o.EntryFeePaise, MaxSlots: o.MaxSlots,
			GrossEntryPaise: o.EntryFeePaise * o.MaxSlots, PercentBp: o.PercentBp, MinFeePaise: o.MinFeePaise,
			MaxFeePaise: o.MaxFeePaise, IsFreeEvent: o.EntryFeePaise == 0, BaseFeePaise: o.BaseFeePaise, GstBp: o.GstBp,
			GstPaise: o.GstPaise, TotalPaise: o.TotalPaise, RefundPolicy: policy, RefundPolicyDocumentID: doc}
		v := Order{OrderID: o.ID, EventID: o.EventID, Status: o.Status, Quote: q, Provider: o.Provider,
			ProviderOrderID: o.ProviderOrderID, RefundPaise: o.RefundPaise, ExpiresAt: o.ExpiresAt, PaidAt: o.PaidAt,
			EventStatus: o.EventStatus}
		if o.Provider == providerRazorpay {
			v.ProviderKeyID = keyID
		}
		out = append(out, v)
	}
	return out, nil
}
