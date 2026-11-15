package appconfig

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/audit"
	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
)

type handler struct{ svc *Service }

// actorOf binds the calling admin and the request to the audit writer.
func actorOf(c *gin.Context) actor {
	return actor{id: authn.AdminID(c), log: func(tx *sql.Tx, action, targetType, targetID string, details any) error {
		return audit.Admin(c, tx, action, targetType, targetID, details)
	}}
}

// respond writes out (with status 200 or 201) or the error.
func respond(c *gin.Context, out any, err error, created bool) {
	switch {
	case err != nil:
		httpx.Fail(c, err)
	case created:
		httpx.Created(c, out)
	default:
		httpx.OK(c, out)
	}
}

// bind decodes the JSON body; on failure it writes the error and returns false.
func bind(c *gin.Context, v any) bool {
	if err := httpx.BindJSON(c, v); err != nil {
		httpx.Fail(c, err)
		return false
	}
	return true
}

// pathID reads :id; on failure it writes the error and returns false.
func pathID(c *gin.Context) (int64, bool) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return 0, false
	}
	return id, true
}

func (h *handler) bootstrap(c *gin.Context) {
	out, err := h.svc.Bootstrap(c.Request.Context())
	respond(c, out, err, false)
}

func (h *handler) policy(c *gin.Context) {
	out, err := h.svc.Policy(c.Request.Context(), c.Param("docType"))
	respond(c, out, err, false)
}

func (h *handler) configList(c *gin.Context) {
	out, err := h.svc.ConfigList(c.Request.Context(), c.Query("category"))
	respond(c, out, err, false)
}

func (h *handler) putConfig(c *gin.Context) {
	var in putConfigBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.PutConfig(c.Request.Context(), actorOf(c), c.Param("key"), in)
	respond(c, out, err, false)
}

func (h *handler) configHistory(c *gin.Context) {
	out, err := h.svc.ConfigHistory(c.Request.Context(), c.Param("key"))
	respond(c, out, err, false)
}

func (h *handler) policies(c *gin.Context) {
	out, err := h.svc.Policies(c.Request.Context(), c.Query("docType"))
	respond(c, out, err, false)
}

func (h *handler) createPolicy(c *gin.Context) {
	var in createPolicyBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreatePolicy(c.Request.Context(), actorOf(c), in)
	respond(c, out, err, true)
}

func (h *handler) activatePolicy(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := h.svc.ActivatePolicy(c.Request.Context(), actorOf(c), id)
	respond(c, out, err, false)
}

func (h *handler) requirements(c *gin.Context) {
	out, err := h.svc.Requirements(c.Request.Context())
	respond(c, out, err, false)
}

func (h *handler) createRequirement(c *gin.Context) {
	var in createReqBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreateRequirement(c.Request.Context(), actorOf(c), in)
	respond(c, out, err, true)
}

func (h *handler) updateRequirement(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in updateReqBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.UpdateRequirement(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, false)
}

func (h *handler) steps(c *gin.Context) {
	out, err := h.svc.Steps(c.Request.Context())
	respond(c, out, err, false)
}

func (h *handler) createStep(c *gin.Context) {
	var in stepBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreateStep(c.Request.Context(), actorOf(c), in)
	respond(c, out, err, true)
}

func (h *handler) updateStep(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in stepBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.UpdateStep(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, false)
}

func (h *handler) sports(c *gin.Context) {
	out, err := h.svc.AdminSports(c.Request.Context())
	respond(c, out, err, false)
}

func (h *handler) createSport(c *gin.Context) {
	var in createSportBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreateSport(c.Request.Context(), actorOf(c), in)
	respond(c, out, err, true)
}

func (h *handler) updateSport(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in updateSportBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.UpdateSport(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, false)
}

func (h *handler) createPosition(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in createPositionBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreatePosition(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, true)
}

func (h *handler) updatePosition(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in updatePositionBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.UpdatePosition(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, false)
}

func (h *handler) createTemplate(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in createTemplateBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.CreateTemplate(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, true)
}

func (h *handler) updateTemplate(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in updateTemplateBody
	if !bind(c, &in) {
		return
	}
	out, err := h.svc.UpdateTemplate(c.Request.Context(), actorOf(c), id, in)
	respond(c, out, err, false)
}
