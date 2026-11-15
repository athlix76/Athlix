package notification

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/push"
)

// Service is the notification business logic.
type Service struct {
	d    *app.Deps
	repo *repository

	mu            sync.Mutex
	sender        push.Sender
	disabledUntil time.Time
}

func newService(d *app.Deps) *Service {
	return &Service{d: d, repo: &repository{db: d.DB}}
}

func (s *Service) now() time.Time {
	if s.d.Now != nil {
		return s.d.Now().UTC()
	}
	return time.Now().UTC()
}

// setSender replaces the push sender (tests inject push.Fake).
func (s *Service) setSender(p push.Sender) {
	s.mu.Lock()
	s.sender = p
	s.mu.Unlock()
}

// pushSender builds the FCM sender lazily from the `fcm` secret. When the secret is missing or
// invalid, push is disabled for 5 minutes and the failure is logged once per period (no detail).
func (s *Service) pushSender(ctx context.Context) push.Sender {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sender != nil {
		return s.sender
	}
	if time.Now().Before(s.disabledUntil) {
		return push.Nop{}
	}
	snd, err := push.FromSecrets(ctx, s.d.Secrets, s.d.Cfg.SecretName(app.SecretFCM))
	if err != nil {
		s.disabledUntil = time.Now().Add(5 * time.Minute)
		logger.From(ctx).Warn("push disabled: fcm secret unavailable")
		return push.Nop{}
	}
	s.sender = snd
	return snd
}

// ---- delivery core ----

func eventHash(e events.Event) string {
	h := sha256.Sum256([]byte(e.Type + "|" + string(e.Payload) + "|" + e.OccurredAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h[:])
}

func dedupeKey(e events.Event, userID int64) string {
	return "notif:evt:" + eventHash(e) + ":" + strconv.FormatInt(userID, 10)
}

// claim marks (event, user) as handled. false means a previous delivery already did it.
func (s *Service) claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.d.Redis.SetNX(ctx, key, "1", ttl).Result()
}

func (s *Service) release(ctx context.Context, keys ...string) {
	if len(keys) == 0 {
		return
	}
	if err := s.d.Redis.Del(ctx, keys...).Err(); err != nil {
		logger.From(ctx).Warn("notification dedupe release failed")
	}
}

// notify writes one in-app row per recipient (once per event, deduped in Redis) and pushes to
// those whose category is on. Row write errors are returned so the event is retried; push
// errors are logged only, because the rows already exist.
func (s *Service) notify(ctx context.Context, e events.Event, recipients []int64, n note) error {
	n.Title, n.Body = clip(n.Title, titleMaxLen), clip(n.Body, bodyMaxLen)
	var fresh []int64
	for _, uid := range uniq(recipients) {
		key := dedupeKey(e, uid)
		ok, err := s.claim(ctx, key, dedupeTTL)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := s.repo.insert(ctx, uid, n); err != nil {
			s.release(ctx, key)
			return err
		}
		fresh = append(fresh, uid)
	}
	if err := s.pushMany(ctx, fresh, n); err != nil {
		logger.From(ctx).Warn("notification push skipped", slog.String("type", n.Type), slog.String("err", "load failed"))
	}
	return nil
}

// pushMany sends n to every device of users whose category pref is on. Dead tokens are
// deleted. It returns an error only when prefs or tokens cannot be loaded.
func (s *Service) pushMany(ctx context.Context, userIDs []int64, n note) error {
	if len(userIDs) == 0 {
		return nil
	}
	for start := 0; start < len(userIDs); start += batchSize {
		chunk := userIDs[start:min(start+batchSize, len(userIDs))]
		targets := chunk
		if n.Category != categoryAccount {
			prefs, err := s.repo.prefs(ctx, chunk)
			if err != nil {
				return err
			}
			targets = targets[:0:0]
			for _, id := range chunk {
				if prefs[id][n.Category] {
					targets = append(targets, id)
				}
			}
		}
		if len(targets) == 0 {
			continue
		}
		devs, err := s.repo.devices(ctx, targets)
		if err != nil {
			return err
		}
		if !s.sendAll(ctx, targets, devs, n) {
			return nil // push disabled
		}
	}
	return nil
}

// sendAll pushes to every device; it returns false when push is disabled.
func (s *Service) sendAll(ctx context.Context, userIDs []int64, devs map[int64][]device, n note) bool {
	var snd push.Sender
	data := map[string]string{"type": n.Type}
	for k, v := range n.Data {
		data[k] = strconv.FormatInt(v, 10)
	}
	for k, v := range n.Links {
		data[k] = v
	}
	log := logger.From(ctx)
	for _, uid := range userIDs {
		for _, dv := range devs[uid] {
			if snd == nil {
				snd = s.pushSender(ctx)
			}
			err := snd.Send(ctx, push.Message{Token: dv.Token, Title: n.Title, Body: n.Body, Data: data})
			switch {
			case err == nil:
			case errors.Is(err, push.ErrDisabled):
				return false
			case errors.Is(err, push.ErrUnregistered):
				if derr := s.repo.deleteDeviceByID(ctx, dv.ID, uid); derr != nil {
					log.Warn("dead device token not deleted", slog.Int64("userId", uid), slog.Int64("deviceId", dv.ID))
				}
			default:
				log.Warn("push send failed", slog.Int64("userId", uid), slog.Int64("deviceId", dv.ID), slog.String("err", err.Error()))
			}
		}
	}
	return true
}

// ---- devices ----

var platforms = []string{"android", "ios", "web"}

func (s *Service) RegisterDevice(ctx context.Context, userID int64, in deviceInput) error {
	in.Platform = strings.TrimSpace(in.Platform)
	in.Token = strings.TrimSpace(in.Token)
	fields := map[string]string{}
	if !slices.Contains(platforms, in.Platform) {
		fields["platform"] = "must be android, ios or web"
	}
	if in.Token == "" || len(in.Token) > 512 {
		fields["token"] = "must be 1 to 512 characters"
	}
	var appVersion any
	if in.AppVersion != nil {
		v := strings.TrimSpace(*in.AppVersion)
		if len(v) > 16 {
			fields["appVersion"] = "must be at most 16 characters"
		}
		appVersion = db.NullString(v)
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return s.repo.upsertDevice(ctx, userID, in.Platform, in.Token, appVersion)
}

func (s *Service) DeleteDevice(ctx context.Context, userID int64, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 512 {
		return httpx.Validation(map[string]string{"token": "must be 1 to 512 characters"})
	}
	return s.repo.deleteDevice(ctx, userID, token)
}

// ---- list / read ----

func (s *Service) List(ctx context.Context, userID int64, p httpx.PageParams) ([]Notification, string, int, error) {
	items, err := s.repo.list(ctx, userID, p.Cursor.ID, p.Limit+1)
	if err != nil {
		return nil, "", 0, err
	}
	next := ""
	if len(items) > p.Limit {
		items = items[:p.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: items[len(items)-1].ID})
	}
	unread, err := s.repo.unreadCount(ctx, userID)
	if err != nil {
		return nil, "", 0, err
	}
	return items, next, unread, nil
}

func (s *Service) MarkRead(ctx context.Context, userID int64, in readInput) error {
	if in.All {
		return s.repo.markAllRead(ctx, userID, s.now())
	}
	if len(in.IDs) == 0 {
		return httpx.Validation(map[string]string{"ids": "send ids or all: true"})
	}
	if len(in.IDs) > readMaxIDs {
		return httpx.Validation(map[string]string{"ids": fmt.Sprintf("at most %d ids", readMaxIDs)})
	}
	for _, id := range in.IDs {
		if id <= 0 {
			return httpx.Validation(map[string]string{"ids": "must be positive ids"})
		}
	}
	return s.repo.markRead(ctx, userID, uniq(in.IDs), s.now())
}

// ---- broadcast ----

var broadcastRoles = []string{"player", "coach", "organiser"}

func validateBroadcast(in *BroadcastInput) error {
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	in.Audience.State = strings.TrimSpace(in.Audience.State)
	fields := map[string]string{}
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > titleMaxLen {
		fields["title"] = "must be 1 to 120 characters"
	}
	if n := utf8.RuneCountInString(in.Body); n < 1 || n > bodyMaxLen {
		fields["body"] = "must be 1 to 500 characters"
	}
	var roles []string
	for _, r := range in.Audience.Roles {
		if !slices.Contains(broadcastRoles, r) {
			fields["audience.roles"] = "must be player, coach or organiser"
			break
		}
		if !slices.Contains(roles, r) {
			roles = append(roles, r)
		}
	}
	in.Audience.Roles = roles
	if in.Audience.SportID < 0 {
		fields["audience.sportId"] = "must be a positive id"
	}
	if utf8.RuneCountInString(in.Audience.State) > 64 {
		fields["audience.state"] = "must be at most 64 characters"
	}
	if len(fields) > 0 {
		return httpx.Validation(fields)
	}
	return nil
}

// Broadcast writes the in-app rows and the audit entry in one transaction, then queues the
// push (marketing category) through EventBroadcastPush.
func (s *Service) Broadcast(c *gin.Context, in BroadcastInput) (BroadcastResult, error) {
	if err := validateBroadcast(&in); err != nil {
		return BroadcastResult{}, err
	}
	ctx := c.Request.Context()
	var n, first, last int64
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		n, first, last, err = s.repo.insertBroadcast(ctx, tx, in)
		if err != nil {
			return err
		}
		return audit.Admin(c, tx, "notification.broadcast", "notification", "", map[string]any{
			"queued": n, "roles": in.Audience.Roles, "sportId": in.Audience.SportID,
			"state": in.Audience.State, "includeMinors": in.Audience.IncludeMinors,
		})
	})
	if err != nil {
		return BroadcastResult{}, err
	}
	if n > 0 {
		ev := events.New(EventBroadcastPush, 0, broadcastPushPayload{FirstID: first, LastID: last, AfterID: first - 1})
		if err := s.d.Events.Publish(ctx, ev); err != nil {
			logger.From(ctx).Error("broadcast push not queued", slog.Int64("firstId", first))
		}
	}
	return BroadcastResult{Queued: n}, nil
}

// onBroadcastPush pushes one batch of broadcast rows to users with marketing on, then queues
// the next batch. Each batch is claimed in Redis so a redelivery does not push twice.
func (s *Service) onBroadcastPush(ctx context.Context, e events.Event) error {
	var p broadcastPushPayload
	if err := e.Decode(&p); err != nil || p.LastID <= 0 {
		return nil
	}
	rows, err := s.repo.broadcastRows(ctx, p.AfterID, p.LastID, batchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	key := fmt.Sprintf("notif:bcast:%d:%d", p.FirstID, p.AfterID)
	claimed, err := s.claim(ctx, key, dedupeTTL)
	if err != nil {
		return err
	}
	if claimed {
		ids := make([]int64, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.UserID)
		}
		n := note{Type: TypeBroadcast, Title: rows[0].Title, Body: rows[0].Body, Category: CategoryMarketing}
		if err := s.pushMany(ctx, ids, n); err != nil {
			s.release(ctx, key)
			return err
		}
	}
	if len(rows) == batchSize {
		next := p
		next.AfterID = rows[len(rows)-1].ID
		return s.d.Events.Publish(ctx, events.New(EventBroadcastPush, 0, next))
	}
	return nil
}

// ---- reminders job ----

// EventReminders reminds registered players notification.event_reminder_hours before start.
// It reads batches of 500 registrations (keyset on registration id) and dedupes per
// (event, user) in Redis, so hourly runs never repeat a reminder.
func (s *Service) EventReminders(ctx context.Context) error {
	hours := s.d.Settings.Int(ctx, "notification.event_reminder_hours", defaultReminderH)
	if hours <= 0 {
		return nil
	}
	now := s.now()
	until := now.Add(time.Duration(hours) * time.Hour)
	ttl := time.Duration(hours)*time.Hour + 48*time.Hour
	var after int64
	for batch := 0; batch < reminderMaxBatch; batch++ {
		rows, err := s.repo.reminderRows(ctx, now, until, after, batchSize)
		if err != nil {
			return err
		}
		byEvent := map[int64][]int64{}
		notes := map[int64]note{}
		var order []int64
		for _, r := range rows {
			after = r.RegID
			key := fmt.Sprintf("notif:reminder:%d:%d", r.EventID, r.UserID)
			ok, err := s.claim(ctx, key, ttl)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			n, seen := notes[r.EventID]
			if !seen {
				n = reminderNote(r, now)
				notes[r.EventID] = n
				order = append(order, r.EventID)
			}
			if err := s.repo.insert(ctx, r.UserID, n); err != nil {
				s.release(ctx, key)
				return err
			}
			byEvent[r.EventID] = append(byEvent[r.EventID], r.UserID)
		}
		for _, ev := range order {
			if err := s.pushMany(ctx, byEvent[ev], notes[ev]); err != nil {
				logger.From(ctx).Warn("reminder push skipped", slog.Int64("eventId", ev))
			}
		}
		if len(rows) < batchSize {
			return nil
		}
	}
	return nil
}

func reminderNote(r reminderRow, now time.Time) note {
	hrs := int(math.Ceil(r.StartsAt.Sub(now).Hours()))
	when := "starts within the hour"
	if hrs > 1 {
		when = fmt.Sprintf("starts in about %d hours", hrs)
	}
	return note{
		Type:     TypeEventReminder,
		Title:    "Event reminder",
		Body:     clip(fmt.Sprintf("%s %s.", quote(r.Title), when), bodyMaxLen),
		Data:     map[string]int64{"eventId": r.EventID},
		Category: CategoryEvents,
	}
}

// ---- helpers ----

func uniq(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func quote(title string) string { return "“" + clip(title, 100) + "”" }

// UnreadCount is GET /notifications/unread-count (the bell badge, without loading a page).
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.repo.unreadCount(ctx, userID)
}
