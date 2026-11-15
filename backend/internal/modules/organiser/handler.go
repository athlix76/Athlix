package organiser

import (
	"database/sql"
	"strconv"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// requireOrganiser allows only users holding the organiser role (403 FORBIDDEN otherwise).
func requireOrganiser(c *gin.Context) {
	if !authn.User(c).HasRole("organiser") {
		httpx.Fail(c, httpx.Forbidden("Only organisers can use organiser verification."))
		return
	}
	c.Next()
}

func (h *handler) get(c *gin.Context) {
	out, err := h.svc.Get(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) save(c *gin.Context) {
	var in saveRequest
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Save(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) submit(c *gin.Context) {
	out, err := h.svc.Submit(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminList(c *gin.Context) {
	page, err := httpx.ParsePage(c, 20)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	status, err := parseStatus(c.Query("status"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, next, err := h.svc.AdminList(c.Request.Context(), status, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Page(c, rows, httpx.Meta{NextCursor: next})
}

func (h *handler) adminDetail(c *gin.Context) {
	id, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.AdminDetail(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// The detail signs the organiser's documents, so the view is audited.
	if err := audit.Admin(c, h.svc.d.DB, "organiser.view", "user", strconv.FormatInt(id, 10), nil); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminApprove(c *gin.Context) { h.review(c, true) }

func (h *handler) adminReject(c *gin.Context) { h.review(c, false) }

func (h *handler) review(c *gin.Context, approve bool) {
	id, err := httpx.PathID(c, "userId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in noteRequest
	if c.Request.ContentLength != 0 {
		if err := httpx.BindJSON(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	note, err := validNote(in.Note, !approve)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	action := "organiser.reject"
	if approve {
		action = "organiser.approve"
	}
	out, err := h.svc.Review(c.Request.Context(), id, authn.AdminID(c), approve, note, func(tx *sql.Tx) error {
		return audit.Admin(c, tx, action, "user", strconv.FormatInt(id, 10), map[string]bool{"withNote": note != nil})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
