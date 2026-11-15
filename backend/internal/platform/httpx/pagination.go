package httpx

import (
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Cursor is an opaque keyset cursor. Most lists page by descending id ("before id"),
// some by (time, id). Clients pass it back unchanged as ?cursor=.
type Cursor struct {
	ID   int64  `json:"i,omitempty"`
	Time string `json:"t,omitempty"`
}

// PageParams are the parsed ?cursor=&limit= values.
type PageParams struct {
	Cursor Cursor
	Limit  int
}

const maxLimit = 100

// ParsePage reads cursor and limit (default def, max 100).
func ParsePage(c *gin.Context, def int) (PageParams, error) {
	p := PageParams{Limit: def}
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return p, Validation(map[string]string{"limit": "must be a positive number"})
		}
		p.Limit = n
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if s := c.Query("cursor"); s != "" {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil || json.Unmarshal(b, &p.Cursor) != nil {
			return p, Validation(map[string]string{"cursor": "is invalid"})
		}
	}
	return p, nil
}

// EncodeCursor returns the opaque string for c.
func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
