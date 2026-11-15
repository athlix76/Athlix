package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"athlix/backend/internal/platform/crypto"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// errWebhookAuth is the one response for any failed webhook authentication (no reason given).
var errWebhookAuth = httpx.Coded(401, httpx.CodeUnauthenticated, "Unauthorized.")

func errWebhookBody() *httpx.Error { return httpx.BadRequest("Malformed notification.") }

// rtdnTypes names Play subscriptionNotification.notificationType values (for the event log).
var rtdnTypes = map[int]string{
	1: "SUBSCRIPTION_RECOVERED", 2: "SUBSCRIPTION_RENEWED", 3: "SUBSCRIPTION_CANCELED",
	4: "SUBSCRIPTION_PURCHASED", 5: "SUBSCRIPTION_ON_HOLD", 6: "SUBSCRIPTION_IN_GRACE_PERIOD",
	7: "SUBSCRIPTION_RESTARTED", 8: "SUBSCRIPTION_PRICE_CHANGE_CONFIRMED", 9: "SUBSCRIPTION_DEFERRED",
	10: "SUBSCRIPTION_PAUSED", 11: "SUBSCRIPTION_PAUSE_SCHEDULE_CHANGED", 12: "SUBSCRIPTION_REVOKED",
	13: "SUBSCRIPTION_EXPIRED", 20: "SUBSCRIPTION_PENDING_PURCHASE_CANCELED",
}

const rtdnRevoked = 12

type pubsubPush struct {
	Message struct {
		Data      string `json:"data"`
		MessageID string `json:"messageId"`
	} `json:"message"`
}

type rtdn struct {
	PackageName              string `json:"packageName"`
	SubscriptionNotification *struct {
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
	} `json:"subscriptionNotification"`
}

// webhookOutcome is the final payment_webhook_events status plus the subscription it touched.
type webhookOutcome struct {
	status string
	subID  int64
	events []events.Event
	ack    func()
}

func ignored() webhookOutcome { return webhookOutcome{status: webhookIgnored} }

// run claims (provider, eventID), runs fn, records the outcome and publishes events after
// commit. Duplicates of a processed/ignored event return nil without calling fn.
func (s *Service) run(ctx context.Context, provider, eventID, eventType string, fn func() (webhookOutcome, error)) error {
	claimed, err := s.repo.claimWebhook(ctx, s.d.DB, provider, eventID, eventType)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	out, err := fn()
	if err != nil {
		_ = s.repo.finishWebhook(ctx, s.d.DB, provider, eventID, webhookFailed, "", s.now())
		return err
	}
	entity := ""
	if out.subID > 0 {
		entity = strconv.FormatInt(out.subID, 10)
	}
	if err := s.repo.finishWebhook(ctx, s.d.DB, provider, eventID, out.status, entity, s.now()); err != nil {
		return err
	}
	s.publish(ctx, out.events)
	if out.ack != nil {
		out.ack()
	}
	return nil
}

// HandlePlayPush handles one Pub/Sub RTDN push. The OIDC token is verified before the body
// is read; the message is only a hint: state is re-fetched from the Play API.
func (s *Service) HandlePlayPush(ctx context.Context, authorization string, readBody func() ([]byte, error)) error {
	if err := s.push.Verify(ctx, authorization); err != nil {
		return errWebhookAuth
	}
	body, err := readBody()
	if err != nil {
		return errWebhookBody()
	}
	var push pubsubPush
	if json.Unmarshal(body, &push) != nil || push.Message.MessageID == "" || len(push.Message.MessageID) > 128 {
		return errWebhookBody()
	}
	data, err := base64.StdEncoding.DecodeString(push.Message.Data)
	var n rtdn
	if err != nil || json.Unmarshal(data, &n) != nil {
		return errWebhookBody()
	}
	eventType := "other"
	if sn := n.SubscriptionNotification; sn != nil {
		eventType = rtdnTypes[sn.NotificationType]
		if eventType == "" {
			eventType = "SUBSCRIPTION_" + strconv.Itoa(sn.NotificationType)
		}
	}
	return s.run(ctx, StoreGooglePlay, push.Message.MessageID, eventType, func() (webhookOutcome, error) {
		return s.applyPlay(ctx, n)
	})
}

func (s *Service) applyPlay(ctx context.Context, n rtdn) (webhookOutcome, error) {
	sn := n.SubscriptionNotification
	if sn == nil || sn.PurchaseToken == "" || len(sn.PurchaseToken) > maxPurchaseTokenLen {
		return ignored(), nil
	}
	pkg, err := s.play.PackageName(ctx)
	if err != nil {
		return webhookOutcome{}, httpx.Upstream("Store is not configured.").Wrap(err)
	}
	if n.PackageName != pkg {
		return ignored(), nil
	}
	ps, err := s.play.Subscription(ctx, sn.PurchaseToken)
	if errors.Is(err, errNotFound) {
		return ignored(), nil
	}
	if err != nil {
		return webhookOutcome{}, httpx.Upstream("Store request failed.").Wrap(err)
	}
	// Find the owner: the row for this token, else (upgrade / resubscribe) the row for the
	// linked token. Either way the purchase must carry that user's account token.
	row, err := s.repo.subByTokenHash(ctx, s.d.DB, crypto.SHA256(sn.PurchaseToken))
	if err != nil {
		return webhookOutcome{}, err
	}
	if row == nil && ps.LinkedPurchaseToken != "" {
		if row, err = s.repo.subByTokenHash(ctx, s.d.DB, crypto.SHA256(ps.LinkedPurchaseToken)); err != nil {
			return webhookOutcome{}, err
		}
	}
	if row == nil || ps.ObfuscatedAccountID == "" || ps.ObfuscatedAccountID != storeAccountToken(s.d.Crypto, row.UserID) {
		return ignored(), nil
	}
	planID := row.PlanID
	if plan, err := s.repo.planByProduct(ctx, s.d.DB, StoreGooglePlay, ps.ProductID); err == nil {
		planID = plan.ID
	} else if !db.IsNoRows(err) {
		return webhookOutcome{}, err
	}
	st := s.playToState(ps, sn.PurchaseToken, planID)
	if sn.NotificationType == rtdnRevoked && !isLive(st.Status) {
		st.Status, st.AutoRenew = StatusRefunded, false
	}
	if ps.State == "SUBSCRIPTION_STATE_PENDING" {
		return ignored(), nil
	}
	saved, evs, err := s.upsert(ctx, row.UserID, st)
	if errors.Is(err, errNotYours) {
		return ignored(), nil
	}
	if err != nil {
		return webhookOutcome{}, err
	}
	out := webhookOutcome{status: webhookProcessed, subID: saved.ID, events: evs}
	if !ps.Acknowledged {
		out.ack = func() { s.acknowledge(ctx, saved, ps.ProductID, sn.PurchaseToken) }
	}
	return out, nil
}

// HandleAppleNotification handles one App Store Server Notification v2. The JWS is verified
// first; the state is re-fetched from the App Store Server API.
func (s *Service) HandleAppleNotification(ctx context.Context, body []byte) error {
	var req struct {
		SignedPayload string `json:"signedPayload"`
	}
	if json.Unmarshal(body, &req) != nil || strings.Count(req.SignedPayload, ".") != 2 {
		return errWebhookAuth
	}
	n, err := s.apple.VerifyNotification(ctx, req.SignedPayload)
	if err != nil || len(n.UUID) > 128 {
		return errWebhookAuth
	}
	return s.run(ctx, StoreAppStore, n.UUID, truncate(n.Type, 64), func() (webhookOutcome, error) {
		return s.applyApple(ctx, n)
	})
}

func (s *Service) applyApple(ctx context.Context, n AppleNotification) (webhookOutcome, error) {
	if !n.Ours || n.TransactionID == "" || n.OriginalTransactionID == "" {
		return ignored(), nil
	}
	row, err := s.repo.subByStoreTxn(ctx, s.d.DB, StoreAppStore, n.OriginalTransactionID)
	if err != nil {
		return webhookOutcome{}, err
	}
	if row == nil {
		return ignored(), nil // purchases are bound to a user only through /premium/purchases/verify
	}
	list, err := s.apple.Statuses(ctx, n.TransactionID)
	if errors.Is(err, errNotFound) {
		return ignored(), nil
	}
	if err != nil {
		return webhookOutcome{}, httpx.Upstream("Store request failed.").Wrap(err)
	}
	var pick *AppleSubscription
	for i := range list {
		if list[i].OriginalTransactionID == n.OriginalTransactionID {
			pick = &list[i]
			break
		}
	}
	if pick == nil || !strings.EqualFold(pick.AppAccountToken, storeAccountToken(s.d.Crypto, row.UserID)) {
		logger.From(ctx).Warn("app store notification not matched", slog.Int64("subscriptionId", row.ID))
		return ignored(), nil
	}
	planID := row.PlanID
	if plan, err := s.repo.planByProduct(ctx, s.d.DB, StoreAppStore, pick.ProductID); err == nil {
		planID = plan.ID
	} else if !db.IsNoRows(err) {
		return webhookOutcome{}, err
	}
	saved, evs, err := s.upsert(ctx, row.UserID, s.appleToState(*pick, planID))
	if err != nil {
		return webhookOutcome{}, err
	}
	return webhookOutcome{status: webhookProcessed, subID: saved.ID, events: evs}, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
