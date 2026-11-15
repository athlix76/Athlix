package listingpayment

import (
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

var ist = time.FixedZone("IST", 5*3600+1800)

func (h *handler) quote(c *gin.Context) {
	in := QuoteInput{EventType: strings.TrimSpace(c.Query("eventType"))}
	var err error
	for _, p := range []struct {
		name string
		dst  *int64
	}{{"sportId", &in.SportID}, {"entryFeePaise", &in.EntryFeePaise}, {"maxSlots", &in.MaxSlots}} {
		if *p.dst, err = httpx.QueryInt64(c, p.name); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	out, err := h.svc.Quote(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) eventQuote(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.EventQuote(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) order(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateOrder(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) latestOrder(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.LatestOrder(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) confirm(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in ConfirmInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Confirm(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) mockPay(c *gin.Context) {
	var in MockPayInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.MockPay(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// webhook reads the raw body (the signature covers the exact bytes) and never logs it.
func (h *handler) webhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBody+1))
	if err != nil || len(body) > maxWebhookBody {
		httpx.Fail(c, httpx.BadRequest("Webhook body is too large."))
		return
	}
	if err := h.svc.Webhook(c.Request.Context(), body, c.GetHeader("X-Razorpay-Signature"), c.GetHeader("X-Razorpay-Event-Id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"received": true})
}

// ---------- admin ----------

// parseTime accepts RFC 3339 or a date (YYYY-MM-DD, IST); endOfDay moves a date to the next midnight.
func parseTime(v string, endOfDay bool) (*time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		t = t.UTC()
		return &t, true
	}
	t, err := time.ParseInLocation("2006-01-02", v, ist)
	if err != nil {
		return nil, false
	}
	if endOfDay {
		t = t.AddDate(0, 0, 1)
	}
	t = t.UTC()
	return &t, true
}

func (h *handler) adminOrders(c *gin.Context) {
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := OrderFilter{Status: strings.TrimSpace(c.Query("status"))}
	if f.EventID, err = httpx.QueryInt64(c, "eventId"); err != nil {
		httpx.Fail(c, err)
		return
	}
	fields := map[string]string{}
	var ok bool
	if f.From, ok = parseTime(c.Query("from"), false); !ok {
		fields["from"] = "must be YYYY-MM-DD or an RFC 3339 time"
	}
	if f.To, ok = parseTime(c.Query("to"), true); !ok {
		fields["to"] = "must be YYYY-MM-DD or an RFC 3339 time"
	}
	if len(fields) > 0 {
		httpx.Fail(c, httpx.Validation(fields))
		return
	}
	items, next, err := h.svc.AdminOrders(c, f, pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) adminRefund(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in RefundInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.AdminRefund(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) feeRules(c *gin.Context) {
	out, err := h.svc.FeeRules(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) createRule(c *gin.Context) {
	var in FeeRuleInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.CreateFeeRule(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateRule(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in FeeRuleInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.UpdateFeeRule(c, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) preview(c *gin.Context) {
	var in QuoteInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Preview(c, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
