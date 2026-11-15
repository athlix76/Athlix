package auth

import (
	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

func (h *handler) requestOTP(c *gin.Context) {
	var in otpRequestBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.RequestOTP(c.Request.Context(), in.Mobile, c.ClientIP())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) verifyOTP(c *gin.Context) {
	var in otpVerifyBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.VerifyOTP(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) refresh(c *gin.Context) {
	var in refreshBody
	if err := httpx.BindJSON(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := h.svc.Refresh(c.Request.Context(), in.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *handler) logout(c *gin.Context) {
	var in refreshBody
	if c.Request.ContentLength != 0 {
		if err := httpx.BindJSON(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if err := h.svc.Logout(c.Request.Context(), authn.UserID(c), in.RefreshToken); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}
