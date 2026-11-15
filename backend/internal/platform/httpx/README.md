# `internal/platform/httpx`

Package httpx holds the HTTP conventions every handler follows: the JSON
envelope, typed API errors, request binding and cursor pagination. See
docs/api.md "Conventions".


## Exported API

```go
func BindJSON(c *gin.Context, dst any) error
func Created(c *gin.Context, v any)
func EncodeCursor(c Cursor) string
func Fail(c *gin.Context, err error)
func NoContent(c *gin.Context)
func OK(c *gin.Context, v any)
func Page(c *gin.Context, items any, meta Meta)
func PathID(c *gin.Context, name string) (int64, error)
func QueryBool(c *gin.Context, name string) bool
func QueryInt64(c *gin.Context, name string) (int64, error)
type Cursor struct
type Error struct
func As(err error) (*Error, bool)
func BadRequest(msg string) *Error
func Coded(status int, code, msg string) *Error
func Conflict(msg string) *Error
func Forbidden(msg string) *Error
func Internal() *Error
func InvalidState(msg string) *Error
func LimitReached(msg string) *Error
func NotFound(what string) *Error
func RateLimited(msg string) *Error
func Unauthenticated() *Error
func Upstream(msg string) *Error
func Validation(fields map[string]string) *Error
func (e *Error) Error() string
func (e *Error) Unwrap() error
func (e *Error) Wrap(cause error) *Error
type Meta struct
type PageParams struct
func ParsePage(c *gin.Context, def int) (PageParams, error)
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/httpx`). Keep the package comment true and update this file when the API changes._
