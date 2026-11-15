package event

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) create(c *gin.Context) {
	var in Input
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), authn.User(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) update(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in Input
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), authn.UserID(c), id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) get(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Get(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// parseFloat reads an optional float query value.
func parseFloat(c *gin.Context, name string, lo, hi float64, fields map[string]string) *float64 {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < lo || v > hi {
		fields[name] = "is out of range"
		return nil
	}
	return &v
}

// parseDate reads YYYY-MM-DD as an IST calendar day; end=true gives the start of the next day.
func parseDate(c *gin.Context, name string, end bool, fields map[string]string) *time.Time {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, ist)
	if err != nil {
		fields[name] = "must be a date (YYYY-MM-DD)"
		return nil
	}
	if end {
		t = t.AddDate(0, 0, 1)
	}
	t = t.UTC()
	return &t
}

func enumQuery(c *gin.Context, name string, allowed map[string]bool, fields map[string]string) string {
	v := strings.TrimSpace(c.Query(name))
	if v != "" && !allowed[v] {
		fields[name] = "is not a valid value"
		return ""
	}
	return v
}

func (h *handler) parseList(c *gin.Context) (ListFilter, error) {
	fields := map[string]string{}
	f := ListFilter{
		Q:             strings.TrimSpace(c.Query("q")),
		State:         strings.TrimSpace(c.Query("state")),
		District:      strings.TrimSpace(c.Query("district")),
		City:          strings.TrimSpace(c.Query("city")),
		EligibleForMe: httpx.QueryBool(c, "eligibleForMe"),
		IncludeClosed: httpx.QueryBool(c, "includeClosed"),
		ShowAll:       httpx.QueryBool(c, "showAll"),
	}
	if len([]rune(f.Q)) > 100 {
		fields["q"] = "is too long"
	}
	var err error
	if f.SportID, err = httpx.QueryInt64(c, "sportId"); err != nil {
		fields["sportId"] = "must be a positive number"
	}
	f.EventType = enumQuery(c, "eventType", eventTypes, fields)
	f.CompetitionLevel = enumQuery(c, "competitionLevel", competitionLevels, fields)
	f.GenderCategory = enumQuery(c, "genderCategory", genderCategories, fields)
	f.Category = enumQuery(c, "category", categories, fields)
	f.From = parseDate(c, "fromDate", false, fields)
	f.To = parseDate(c, "toDate", true, fields)
	if c.Query("maxEntryFeePaise") != "" {
		v, err := httpx.QueryInt64(c, "maxEntryFeePaise")
		if err != nil {
			fields["maxEntryFeePaise"] = "must be a positive number"
		} else {
			f.MaxEntryFeePaise = &v
		}
	}
	f.Lat = parseFloat(c, "lat", -90, 90, fields)
	f.Lng = parseFloat(c, "lng", -180, 180, fields)
	hasPoint := f.Lat != nil && f.Lng != nil
	if (c.Query("lat") == "") != (c.Query("lng") == "") {
		fields["lat"] = "lat and lng go together"
	}
	if s := strings.TrimSpace(c.Query("radiusKm")); s != "" {
		maxKm := h.svc.d.Settings.Int(c.Request.Context(), "event.max_search_radius_km", defMaxRadiusKm)
		n, err := strconv.Atoi(s)
		switch {
		case err != nil || n < 1 || n > maxKm:
			fields["radiusKm"] = "must be between 1 and " + strconv.Itoa(maxKm)
		case !hasPoint:
			fields["radiusKm"] = "needs lat and lng"
		default:
			f.RadiusKm = n
		}
	}
	switch c.DefaultQuery("sort", "startsAt") {
	case "startsAt":
	case "distance":
		if !hasPoint {
			fields["sort"] = "distance needs lat and lng"
		}
		f.SortDistance = true
	default:
		fields["sort"] = "must be startsAt or distance"
	}
	if len(fields) > 0 {
		return f, httpx.Validation(fields)
	}
	return f, nil
}

func (h *handler) list(c *gin.Context) {
	f, err := h.parseList(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.List(c.Request.Context(), authn.UserID(c), f, pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) featured(c *gin.Context) {
	sportID, err := httpx.QueryInt64(c, "sportId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Featured(c.Request.Context(), authn.UserID(c), sportID, strings.TrimSpace(c.Query("state")), httpx.QueryBool(c, "showAll"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) preview(c *gin.Context) {
	in := EligibilityInput{Type: strings.TrimSpace(c.Query("type")), Scope: strings.TrimSpace(c.Query("scope"))}
	fields := map[string]string{}
	var err error
	if in.DistrictID, err = httpx.QueryInt64(c, "districtId"); err != nil {
		fields["eligibility.districtId"] = "must be a positive number"
	}
	if in.StateID, err = httpx.QueryInt64(c, "stateId"); err != nil {
		fields["eligibility.stateId"] = "must be a positive number"
	}
	if s := strings.TrimSpace(c.Query("requireVerified")); s != "" {
		v := httpx.QueryBool(c, "requireVerified")
		in.RequireVerified = &v
	}
	out := h.svc.Preview(c.Request.Context(), strings.TrimSpace(c.Query("category")), in)
	for k, v := range fields {
		out.Fields[k] = v
		out.Valid = false
	}
	httpx.OK(c, out)
}

func (h *handler) mine(c *gin.Context) {
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.Mine(c.Request.Context(), authn.UserID(c), strings.TrimSpace(c.Query("status")), pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) contact(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Contact(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) cancel(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in ReasonInput
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Cancel(c.Request.Context(), authn.UserID(c), id, in.Reason)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) feature(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Feature(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Admin handlers.

func (h *handler) adminList(c *gin.Context) {
	sportID, err := httpx.QueryInt64(c, "sportId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	pg, err := httpx.ParsePage(c, defPageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := AdminFilter{Status: strings.TrimSpace(c.Query("status")), SportID: sportID, Q: strings.TrimSpace(c.Query("q"))}
	items, next, err := h.svc.AdminList(c, f, pg)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) adminGet(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.AdminGet(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminApprove(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Approve(c, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminFeature(on bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.PathID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := h.svc.AdminFeature(c, id, on)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func (h *handler) adminReason(move func(*gin.Context, int64, string) (AdminEvent, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.PathID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in ReasonInput
		if err := httpx.BindJSON(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := move(c, id, in.Reason)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}
