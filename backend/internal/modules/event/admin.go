package event

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/db"
	"athlix/backend/internal/platform/events"
	"athlix/backend/internal/platform/httpx"
)

// Admin review (permission events.review). Every write runs audit.Admin in the same tx and
// publishes after commit.

// AdminList is GET /admin/events.
func (s *Service) AdminList(c *gin.Context, f AdminFilter, pg httpx.PageParams) ([]AdminSummary, string, error) {
	ctx := c.Request.Context()
	if f.Status != "" && !statuses[f.Status] {
		return nil, "", httpx.Validation(map[string]string{"status": "is not a valid status"})
	}
	rs, err := s.r.adminList(ctx, f, pg)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rs) > pg.Limit {
		rs = rs[:pg.Limit]
		next = httpx.EncodeCursor(httpx.Cursor{ID: rs[len(rs)-1].ID})
	}
	sums, err := s.summaries(ctx, nil, rs, true)
	if err != nil {
		return nil, "", err
	}
	ids := make([]int64, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	ls, err := s.r.latestListing(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]AdminSummary, len(sums))
	for i, sm := range sums {
		out[i] = AdminSummary{Summary: sm}
		if l, ok := ls[sm.ID]; ok {
			l := l
			out[i].Listing = &l
		}
	}
	return out, next, nil
}

// AdminGet is GET /admin/events/:id.
func (s *Service) AdminGet(c *gin.Context, id int64) (AdminEvent, error) {
	ctx := c.Request.Context()
	r, err := s.r.get(ctx, nil, id, false)
	if err != nil {
		return AdminEvent{}, err
	}
	ev, err := s.detail(ctx, 0, r, true)
	if err != nil {
		return AdminEvent{}, err
	}
	orders, err := s.r.listingOrders(ctx, id, r.Status)
	if err != nil {
		return AdminEvent{}, err
	}
	if orders == nil {
		orders = []ListingOrder{}
	}
	return AdminEvent{Event: ev, ListingOrders: orders}, nil
}

func (s *Service) adminMove(c *gin.Context, id int64, from, to, action, reason, evType string) (AdminEvent, error) {
	ctx := c.Request.Context()
	var r rec
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		var err error
		if r, err = s.r.get(ctx, tx, id, true); err != nil {
			return err
		}
		if r.Status != from {
			return httpx.InvalidState("The event is " + r.Status + ".")
		}
		if err := s.r.transition(ctx, tx, id, []string{from}, to, reason, s.now()); err != nil {
			return err
		}
		return audit.Admin(c, tx, action, "event", strconv.FormatInt(id, 10), map[string]any{"from": from, "to": to})
	})
	if err != nil {
		return AdminEvent{}, err
	}
	s.publish(ctx, evType, authn.AdminID(c), r, reason)
	return s.AdminGet(c, id)
}

// Approve is POST /admin/events/:id/approve (pending_review → published).
func (s *Service) Approve(c *gin.Context, id int64) (AdminEvent, error) {
	return s.adminMove(c, id, stPendingReview, stPublished, "event.approve", "", events.EventPublished)
}

func adminReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return "", httpx.Validation(map[string]string{"reason": "is required (max 500 characters)"})
	}
	return reason, nil
}

// Reject is POST /admin/events/:id/reject (pending_review → rejected; listingpayment refunds).
func (s *Service) Reject(c *gin.Context, id int64, reason string) (AdminEvent, error) {
	reason, err := adminReason(reason)
	if err != nil {
		return AdminEvent{}, err
	}
	return s.adminMove(c, id, stPendingReview, stRejected, "event.reject", reason, events.EventRejected)
}

// Remove is POST /admin/events/:id/remove (published → removed; emits event.cancelled).
func (s *Service) Remove(c *gin.Context, id int64, reason string) (AdminEvent, error) {
	reason, err := adminReason(reason)
	if err != nil {
		return AdminEvent{}, err
	}
	return s.adminMove(c, id, stPublished, stRemoved, "event.remove", reason, events.EventCancelled)
}

// AdminFeature is POST /admin/events/:id/feature (on) and /unfeature (off). Featuring needs a
// published event; repeating either call is a no-op without an audit row.
func (s *Service) AdminFeature(c *gin.Context, id int64, on bool) (AdminEvent, error) {
	ctx := c.Request.Context()
	err := db.WithTx(ctx, s.d.DB, func(tx *sql.Tx) error {
		r, err := s.r.get(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if (r.FeaturedAt != nil) == on {
			return nil
		}
		action := "event.unfeature"
		if on {
			if r.Status != stPublished {
				return httpx.InvalidState("Only published events can be featured.")
			}
			action = "event.feature"
			err = s.r.setFeatured(ctx, tx, id, s.now())
		} else {
			err = s.r.clearFeatured(ctx, tx, id)
		}
		if err != nil {
			return err
		}
		return audit.Admin(c, tx, action, "event", strconv.FormatInt(id, 10), nil)
	})
	if err != nil {
		return AdminEvent{}, err
	}
	return s.AdminGet(c, id)
}
