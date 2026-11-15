package sport

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ d *app.Deps }

func (h *handler) list(c *gin.Context) {
	out, err := Catalogue(c.Request.Context(), h.d.DB, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
