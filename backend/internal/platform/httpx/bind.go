package httpx

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// BindJSON decodes the body into dst. It does not validate business rules; call the
// service's own validate() after binding. Unknown fields are ignored.
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return BadRequest("Request body is not valid JSON for this endpoint.")
	}
	return nil
}

// PathID parses a positive int64 path parameter (e.g. ":id").
func PathID(c *gin.Context, name string) (int64, error) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || v <= 0 {
		return 0, NotFound("Resource")
	}
	return v, nil
}

// QueryInt64 parses an optional int64 query parameter; 0 when absent.
func QueryInt64(c *gin.Context, name string) (int64, error) {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, Validation(map[string]string{name: "must be a positive number"})
	}
	return v, nil
}

// QueryBool parses "true"/"1" as true.
func QueryBool(c *gin.Context, name string) bool {
	s := strings.ToLower(strings.TrimSpace(c.Query(name)))
	return s == "true" || s == "1"
}
