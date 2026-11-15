# `internal/testutil`

Package testutil builds a fully wired *app.Deps for unit tests with no network:
sqlmock DB, miniredis, static settings, random crypto/JWT keys, in-memory
storage, synchronous event bus, stub SMS.

d, mock := testutil.Deps(t, map[string]any{"chat.group_max_members": 3})
mock.ExpectQuery(`SELECT .* FROM events WHERE id = \? AND organiser_id = \?`).WithArgs(7, 42).WillReturnRows(...)


## Exported API

```go
func AdminEngine(adminID int64, perms []string) (*gin.Engine, app.Routes)
func Deps(t *testing.T, cfgValues map[string]any) (*app.Deps, sqlmock.Sqlmock)
func Request(r http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder
func UserEngine(u userdir.Basic) (*gin.Engine, app.Routes)
func VerifiedUser(id int64) userdir.Basic
type MemStorage struct
func NewMemStorage() *MemStorage
func (m *MemStorage) Delete(_ context.Context, key string) error
func (m *MemStorage) Head(_ context.Context, key string) (storage.Object, error)
func (m *MemStorage) PresignGet(_ context.Context, key string, _ time.Duration) (string, error)
func (m *MemStorage) PresignPut(_ context.Context, key, ct string, size int64, _ time.Duration) (string, map[string]string, error)
func (m *MemStorage) Put(key, contentType string, size int64)
type RecordingSMS struct
func (r *RecordingSMS) SendOTP(_ context.Context, template, mobile, otp string) error
type SentSMS struct{ Template, Mobile, OTP string }
```

_Generated from the Go doc comments (`go doc -all ./internal/testutil`). Keep the package comment true and update this file when the API changes._
