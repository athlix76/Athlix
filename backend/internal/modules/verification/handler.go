package verification

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) start(c *gin.Context) {
	res, err := h.svc.Start(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

// callback is public: the one-time state binds it to a user. It always redirects to the app;
// the query (code, state) is never logged or echoed.
func (h *handler) callback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusFound, h.svc.Callback(c.Request.Context(), c.Request.URL.Query()))
}

func (h *handler) status(c *gin.Context) {
	st, err := h.svc.Status(c.Request.Context(), authn.UserID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, st)
}

func (h *handler) mock(c *gin.Context) {
	var in mockBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	st, err := h.svc.Mock(c.Request.Context(), authn.UserID(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, st)
}
