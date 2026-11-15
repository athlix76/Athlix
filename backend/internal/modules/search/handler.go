package search

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) people(c *gin.Context) {
	p, err := httpx.ParsePage(c, h.svc.PageSize(c.Request.Context()))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f, err := parsePeople(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, next, err := h.svc.People(c.Request.Context(), authn.UserID(c), f, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, items, httpx.Meta{NextCursor: next})
}

func (h *handler) global(c *gin.Context) {
	out, err := h.svc.Global(c.Request.Context(), authn.UserID(c), c.Query("q"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func parsePeople(c *gin.Context) (PeopleFilter, error) {
	f := PeopleFilter{
		Q:                c.Query("q"),
		Role:             strings.TrimSpace(c.Query("role")),
		State:            c.Query("state"),
		District:         c.Query("district"),
		City:             c.Query("city"),
		AchievementLevel: strings.TrimSpace(c.Query("achievementLevel")),
	}
	var err error
	if f.SportID, err = httpx.QueryInt64(c, "sportId"); err != nil {
		return f, err
	}
	if f.PositionID, err = httpx.QueryInt64(c, "positionId"); err != nil {
		return f, err
	}
	if f.AgeMin, err = queryInt(c, "ageMin"); err != nil {
		return f, err
	}
	if f.AgeMax, err = queryInt(c, "ageMax"); err != nil {
		return f, err
	}
	return f, nil
}

// queryInt parses an optional integer query parameter; nil when absent.
func queryInt(c *gin.Context, name string) (*int, error) {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, httpx.Validation(map[string]string{name: "must be a whole number"})
	}
	return &n, nil
}
