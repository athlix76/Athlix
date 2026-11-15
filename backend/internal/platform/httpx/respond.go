package httpx

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/logger"
)

// Meta carries pagination info. NextCursor is empty on the last page.
type Meta struct {
	NextCursor  string `json:"nextCursor,omitempty"`
	UnreadCount *int   `json:"unreadCount,omitempty"`
	Total       *int   `json:"total,omitempty"`
}

type envelope struct {
	Data any   `json:"data"`
	Meta *Meta `json:"meta,omitempty"`
}

// OK writes 200 {"data": v}.
func OK(c *gin.Context, v any) { c.JSON(http.StatusOK, envelope{Data: v}) }

// Created writes 201 {"data": v}.
func Created(c *gin.Context, v any) { c.JSON(http.StatusCreated, envelope{Data: v}) }

// Page writes 200 {"data": items, "meta": meta}. items must never be nil (use empty slice).
func Page(c *gin.Context, items any, meta Meta) {
	c.JSON(http.StatusOK, envelope{Data: items, Meta: &meta})
}

// NoContent writes 204.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// Fail writes the error envelope. Unknown errors become 500 and are logged with the cause;
// the client only ever sees the safe message.
func Fail(c *gin.Context, err error) {
	e, ok := As(err)
	if !ok {
		e = Internal().Wrap(err)
	}
	log := logger.From(c.Request.Context())
	if e.Status >= 500 {
		log.Error("request failed", slog.String("code", e.Code), slog.String("err", errString(e)))
	} else {
		log.Info("request rejected", slog.String("code", e.Code), slog.Int("status", e.Status))
	}
	c.AbortWithStatusJSON(e.Status, gin.H{"error": e})
}

func errString(e *Error) string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.Message
}
