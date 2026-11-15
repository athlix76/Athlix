package media

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) create(c *gin.Context) {
	var in uploadBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	t, err := h.svc.Create(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, t)
}

func (h *handler) confirm(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Confirm(c.Request.Context(), authn.UserID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) adminCreate(c *gin.Context) {
	var in uploadBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	t, err := h.svc.CreateAdmin(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// platform/media inserts outside a caller transaction, so the audit row follows it.
	if err := audit.Admin(c, h.svc.d.DB, "media.upload_create", "media_upload", strconv.FormatInt(t.UploadID, 10), map[string]any{"purpose": in.Purpose}); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, t)
}

func (h *handler) adminConfirm(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.ConfirmAdmin(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := audit.Admin(c, h.svc.d.DB, "media.upload_confirm", "media_upload", strconv.FormatInt(out.UploadID, 10), nil); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
