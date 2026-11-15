package subscription

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Service holds the subscription business rules.
type Service struct {
	d     *app.Deps
	repo  repo
	play  PlayStore
	push  PushVerifier
	apple AppleStore
}

func newService(d *app.Deps) *Service {
	hc := &http.Client{Timeout: 10 * time.Second}
	now := func() time.Time { return d.Now().UTC() }
	return &Service{
		d: d,
		play: &playClient{secrets: d.Secrets, name: d.Cfg.SecretName(app.SecretGooglePlay), http: hc,
			tokenURL: googleTokenURL, apiBase: googlePlayAPI, now: now},
		push: &pubsubVerifier{secrets: d.Secrets, name: d.Cfg.SecretName(app.SecretGooglePlay), http: hc,
			certsURL: googleCertsURL, now: now},
		apple: &appleClient{secrets: d.Secrets, name: d.Cfg.SecretName(app.SecretAppStore), http: hc,
			bases: appleAPIBase, jws: newAppleJWSVerifier(now), now: now},
	}
}

func (s *Service) now() time.Time { return s.d.Now().UTC() }

// storeAccountToken is the value the app passes as Play obfuscatedAccountId / Apple
// appAccountToken: the first 16 bytes of Box.Hash("store_account", "<userID>") with the UUID
// version-4 and RFC 4122 variant bits set. It must stay byte-for-byte equal to the user
// module's StoreAccountToken (returned to the app as Me.storeAccountToken).
func storeAccountToken(box *crypto.Box, userID int64) string {
	return box.StoreAccountToken(userID)
}

var (
	errPurchaseNotFound = httpx.NotFound("Purchase")
	errPlanNotFound     = httpx.NotFound("Plan")
	appleTxnIDPattern   = regexp.MustCompile(`^[0-9]{1,64}$`)
)

const maxPurchaseTokenLen = 4096

// ---- reads ----

// Plans lists active plans for the app, or [] when premium.enabled is false.
func (s *Service) Plans(ctx context.Context, audience string) ([]Plan, error) {
	if audience != "" && !audiences[audience] {
		return nil, httpx.Validation(map[string]string{"audience": "must be player, coach or organiser"})
	}
	out := []Plan{}
	if !s.d.Settings.Bool(ctx, "premium.enabled", false) {
		return out, nil
	}
	plans, err := s.repo.listPlans(ctx, s.d.DB, audience, true)
	if err != nil {
		return nil, err
	}
	if err := s.repo.withBenefits(ctx, s.d.DB, plans); err != nil {
		return nil, err
	}
	for _, p := range plans {
		out = append(out, p.Plan)
	}
	return out, nil
}

// Mine returns the caller's current subscription and merged benefits.
func (s *Service) Mine(ctx context.Context, userID int64) (MySubscription, error) {
	out := MySubscription{Benefits: map[string]json.RawMessage{}}
	row, err := s.repo.currentForUser(ctx, s.d.DB, userID, s.now())
	if err != nil {
		return out, err
	}
	if row != nil {
		subs, err := s.render(ctx, s.d.DB, []subRow{*row})
		if err != nil {
			return out, err
		}
		out.Active = &subs[0]
	}
	b, err := s.d.Entitlements.Benefits(ctx, userID)
	if err != nil {
		return out, err
	}
	for k, v := range b {
		out.Benefits[k] = v
	}
	return out, nil
}

// render turns rows into API Subscriptions (plans with benefits embedded).
func (s *Service) render(ctx context.Context, q db.Querier, rows []subRow) ([]Subscription, error) {
	seen := map[int64]bool{}
	var plans []AdminPlan
	for _, r := range rows {
		if seen[r.PlanID] {
			continue
		}
		seen[r.PlanID] = true
		p, err := s.repo.getPlan(ctx, q, r.PlanID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	if err := s.repo.withBenefits(ctx, q, plans); err != nil {
		return nil, err
	}
	byID := map[int64]Plan{}
	for _, p := range plans {
		byID[p.ID] = p.Plan
	}
	out := make([]Subscription, 0, len(rows))
	for _, r := range rows {
		out = append(out, Subscription{ID: r.ID, Plan: byID[r.PlanID], Store: r.Store, Status: r.Status,
			AutoRenew: r.AutoRenew, StartedAt: r.StartedAt.UTC(), ExpiresAt: r.ExpiresAt.UTC()})
	}
	return out, nil
}

func (s *Service) renderOne(ctx context.Context, r subRow) (Subscription, error) {
	subs, err := s.render(ctx, s.d.DB, []subRow{r})
	if err != nil {
		return Subscription{}, err
	}
	return subs[0], nil
}

// ---- store purchases ----

// storeState is what a store says about one purchase, mapped to our columns.
type storeState struct {
	Store     string
	TxnID     string
	TokenHash []byte
	PlanID    int64
	Status    string
	AutoRenew bool
	StartedAt time.Time
	ExpiresAt time.Time
	// ReplacesTokenHash is the Play linkedPurchaseToken hash (upgrade / resubscribe): that
	// older row of the same user is cancelled.
	ReplacesTokenHash []byte
}

var errNotYours = errors.New("subscription belongs to another user")

// Verify checks a purchase with the store and records it for the caller. Store purchases are
// honoured even when premium.enabled is false (the user has already paid).
func (s *Service) Verify(ctx context.Context, userID int64, req verifyRequest) (Subscription, error) {
	req.ProductID = strings.TrimSpace(req.ProductID)
	f := map[string]string{}
	if req.ProductID == "" || len(req.ProductID) > 128 {
		f["productId"] = "is required"
	}
	switch req.Store {
	case StoreGooglePlay:
		if req.PurchaseToken == "" || len(req.PurchaseToken) > maxPurchaseTokenLen {
			f["purchaseToken"] = "is required for google_play"
		}
	case StoreAppStore:
		if !appleTxnIDPattern.MatchString(req.TransactionID) {
			f["transactionId"] = "is required for app_store"
		}
	default:
		f["store"] = "must be google_play or app_store"
	}
	if len(f) > 0 {
		return Subscription{}, httpx.Validation(f)
	}
	var st storeState
	var err error
	if req.Store == StoreGooglePlay {
		st, err = s.playState(ctx, userID, req.ProductID, req.PurchaseToken)
	} else {
		st, err = s.appleState(ctx, userID, req.ProductID, req.TransactionID)
	}
	if err != nil {
		return Subscription{}, err
	}
	row, evs, err := s.upsert(ctx, userID, st)
	if errors.Is(err, errNotYours) {
		logger.From(ctx).Warn("store purchase claimed by another user", slog.Int64("userId", userID))
		return Subscription{}, errPurchaseNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	s.publish(ctx, evs)
	if req.Store == StoreGooglePlay {
		s.acknowledge(ctx, row, req.ProductID, req.PurchaseToken)
	}
	return s.renderOne(ctx, row)
}

// storeErr maps a store client error to an API error.
func storeErr(err error) error {
	switch {
	case errors.Is(err, errNotFound):
		return errPurchaseNotFound
	case errors.Is(err, errStoreDisabled):
		return httpx.InvalidState("Purchases from this store are not available right now.")
	default:
		return httpx.Upstream("Could not verify the purchase with the store. Please try again.").Wrap(err)
	}
}

func (s *Service) playState(ctx context.Context, userID int64, productID, token string) (storeState, error) {
	ps, err := s.play.Subscription(ctx, token)
	if err != nil {
		return storeState{}, storeErr(err)
	}
	if ps.ProductID != productID || ps.ObfuscatedAccountID == "" || ps.ObfuscatedAccountID != storeAccountToken(s.d.Crypto, userID) {
		return storeState{}, errPurchaseNotFound
	}
	if ps.State == "SUBSCRIPTION_STATE_PENDING" {
		return storeState{}, httpx.InvalidState("The payment is still pending with Google Play.")
	}
	plan, err := s.repo.planByProduct(ctx, s.d.DB, StoreGooglePlay, ps.ProductID)
	if db.IsNoRows(err) {
		return storeState{}, httpx.Validation(map[string]string{"productId": "is not a known plan"})
	}
	if err != nil {
		return storeState{}, err
	}
	return s.playToState(ps, token, plan.ID), nil
}

// playTxnID is the stable store_transaction_id of a Play purchase: hex SHA-256 of its purchase
// token (the token is constant across renewals; order ids change with every renewal).
func playTxnID(tokenHash []byte) string { return hex.EncodeToString(tokenHash) }

func (s *Service) playToState(ps PlaySubscription, token string, planID int64) storeState {
	h := crypto.SHA256(token)
	st := storeState{Store: StoreGooglePlay, TxnID: playTxnID(h), TokenHash: h, PlanID: planID,
		Status: playStatus(ps.State, ps.ExpiresAt, s.now()), AutoRenew: ps.AutoRenew,
		StartedAt: ps.StartedAt, ExpiresAt: ps.ExpiresAt}
	if st.StartedAt.IsZero() {
		st.StartedAt = s.now()
	}
	if ps.LinkedPurchaseToken != "" {
		st.ReplacesTokenHash = crypto.SHA256(ps.LinkedPurchaseToken)
	}
	return st
}

// playStatus maps subscriptionsv2 subscriptionState. Unknown states give no access.
func playStatus(state string, expires, now time.Time) string {
	switch state {
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_CANCELED": // canceled = runs to the end of the period
		if expires.After(now) {
			return StatusActive
		}
	case "SUBSCRIPTION_STATE_IN_GRACE_PERIOD":
		if expires.After(now) {
			return StatusGrace
		}
	}
	return StatusExpired
}

func (s *Service) appleState(ctx context.Context, userID int64, productID, txnID string) (storeState, error) {
	list, err := s.apple.Statuses(ctx, txnID)
	if err != nil {
		return storeState{}, storeErr(err)
	}
	var pick *AppleSubscription
	for i := range list {
		if list[i].ProductID == productID {
			pick = &list[i]
			break
		}
	}
	if pick == nil || pick.AppAccountToken == "" || !strings.EqualFold(pick.AppAccountToken, storeAccountToken(s.d.Crypto, userID)) {
		return storeState{}, errPurchaseNotFound
	}
	plan, err := s.repo.planByProduct(ctx, s.d.DB, StoreAppStore, pick.ProductID)
	if db.IsNoRows(err) {
		return storeState{}, httpx.Validation(map[string]string{"productId": "is not a known plan"})
	}
	if err != nil {
		return storeState{}, err
	}
	return s.appleToState(*pick, plan.ID), nil
}

func (s *Service) appleToState(a AppleSubscription, planID int64) storeState {
	st := storeState{Store: StoreAppStore, TxnID: a.OriginalTransactionID, PlanID: planID,
		AutoRenew: a.AutoRenew, StartedAt: a.StartedAt, ExpiresAt: a.ExpiresAt}
	now := s.now()
	switch {
	case a.Revoked || a.Status == 5:
		st.Status, st.AutoRenew = StatusRefunded, false
	case a.Status == 1 && a.ExpiresAt.After(now):
		st.Status = StatusActive
	case a.Status == 4:
		if a.GraceExpiresAt != nil && a.GraceExpiresAt.After(st.ExpiresAt) {
			st.ExpiresAt = *a.GraceExpiresAt
		}
		st.Status = StatusExpired
		if st.ExpiresAt.After(now) {
			st.Status = StatusGrace
		}
	default:
		st.Status = StatusExpired
	}
	if st.StartedAt.IsZero() || st.StartedAt.Unix() == 0 {
		st.StartedAt = now
	}
	return st
}

// upsert writes st for userID in one transaction and returns the events to publish after commit.
func (s *Service) upsert(ctx context.Context, userID int64, st storeState) (subRow, []events.Event, error) {
	var out subRow
	var evs []events.Event
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		evs = nil
		row, err := s.repo.subByStoreTxn(ctx, tx, st.Store, st.TxnID)
		if err != nil {
			return err
		}
		if row != nil && row.UserID != userID {
			return errNotYours
		}
		txn := st.TxnID
		next := subRow{UserID: userID, PlanID: st.PlanID, Store: st.Store, StoreTransactionID: &txn,
			Status: st.Status, AutoRenew: st.AutoRenew, StartedAt: st.StartedAt, ExpiresAt: st.ExpiresAt}
		if row == nil {
			if next.ID, err = s.repo.insertSub(ctx, tx, next, st.TokenHash); err != nil {
				if db.IsDuplicate(err) {
					return httpx.Conflict("This purchase is already being recorded. Please retry.")
				}
				return err
			}
		} else {
			next.ID, next.StartedAt = row.ID, row.StartedAt
			if err := s.repo.updateSubState(ctx, tx, next); err != nil {
				return err
			}
		}
		evs = append(evs, transition(row, next)...)
		if st.ReplacesTokenHash != nil {
			old, err := s.repo.subByTokenHash(ctx, tx, st.ReplacesTokenHash)
			if err != nil {
				return err
			}
			if old != nil && old.ID != next.ID && old.UserID == userID && isLive(old.Status) {
				if err := s.repo.setStatus(ctx, tx, old.ID, StatusCancelled); err != nil {
					return err
				}
				evs = append(evs, subEvent(events.SubscriptionExpired, *old))
			}
		}
		out = next
		return nil
	})
	return out, evs, err
}

// transition returns activated/expired when a row crosses the live boundary.
func transition(before *subRow, after subRow) []events.Event {
	was := before != nil && isLive(before.Status)
	now := isLive(after.Status)
	switch {
	case !was && now:
		return []events.Event{subEvent(events.SubscriptionActivated, after)}
	case was && !now:
		return []events.Event{subEvent(events.SubscriptionExpired, after)}
	}
	return nil
}

func subEvent(typ string, r subRow) events.Event {
	return events.New(typ, r.UserID, events.SubscriptionPayload{SubscriptionID: r.ID, UserID: r.UserID, PlanID: r.PlanID})
}

func (s *Service) publish(ctx context.Context, evs []events.Event) {
	for _, e := range evs {
		if err := s.d.Events.Publish(ctx, e); err != nil {
			logger.From(ctx).Error("publish failed", slog.String("type", e.Type))
		}
	}
}

// acknowledge tells Play we granted the purchase (unacknowledged purchases are refunded after
// 3 days). Failure is logged only: the app also acknowledges, and the next RTDN retries.
func (s *Service) acknowledge(ctx context.Context, row subRow, productID, token string) {
	if !isLive(row.Status) {
		return
	}
	if err := s.play.Acknowledge(ctx, productID, token); err != nil {
		logger.From(ctx).Warn("play acknowledge failed", slog.Int64("subscriptionId", row.ID))
	}
}

// ---- manual subscriptions ----

// periodEnd is start + one billing period.
func periodEnd(start time.Time, period string) time.Time {
	return start.AddDate(0, billingPeriods[period], 0)
}

// MockActivate gives the caller one billing period of planID without a store (ENV≠prod only).
func (s *Service) MockActivate(ctx context.Context, userID int64, planID int64) (Subscription, error) {
	if s.d.Cfg.IsProd() {
		return Subscription{}, httpx.NotFound("Route")
	}
	if planID <= 0 {
		return Subscription{}, httpx.Validation(map[string]string{"planId": "is required"})
	}
	if !s.d.Settings.Bool(ctx, "premium.enabled", false) {
		return Subscription{}, httpx.InvalidState("Premium is not available right now.")
	}
	plan, err := s.repo.getPlan(ctx, s.d.DB, planID)
	if db.IsNoRows(err) || (err == nil && !plan.Active) {
		return Subscription{}, errPlanNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	now := s.now()
	row := subRow{UserID: userID, PlanID: plan.ID, Store: StoreManual, Status: StatusActive, StartedAt: now,
		ExpiresAt: periodEnd(now, plan.BillingPeriod)}
	if row.ID, err = s.repo.insertSub(ctx, s.d.DB, row, nil); err != nil {
		return Subscription{}, err
	}
	s.publish(ctx, []events.Event{subEvent(events.SubscriptionActivated, row)})
	return s.renderOne(ctx, row)
}

// ---- job ----

// ExpireDue moves live subscriptions past expires_at to expired (one batch) and emits
// subscription.expired per row after commit. Safe to re-run.
func (s *Service) ExpireDue(ctx context.Context) error {
	now := s.now()
	var due []subRow
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if due, err = s.repo.dueForExpiry(ctx, tx, now, jobBatch); err != nil || len(due) == 0 {
			return err
		}
		ids := make([]int64, len(due))
		for i, r := range due {
			ids[i] = r.ID
		}
		return s.repo.expire(ctx, tx, ids, now)
	})
	if err != nil {
		return err
	}
	evs := make([]events.Event, 0, len(due))
	for _, r := range due {
		evs = append(evs, subEvent(events.SubscriptionExpired, r))
	}
	s.publish(ctx, evs)
	if len(due) > 0 {
		logger.From(ctx).Info("subscriptions expired", slog.Int("count", len(due)))
	}
	return nil
}
