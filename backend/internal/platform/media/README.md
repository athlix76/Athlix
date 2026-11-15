# `internal/platform/media`

Package media owns the media_uploads table and EVERY object key in the media
bucket.

Flow (docs/flows/media-upload.md):
 1. App: POST /v1/media/uploads {purpose, contentType, sizeBytes} ->
CreateUpload builds the key "<purpose>/<userId-from-token>/<uuid>.<ext>",
returns {uploadId, uploadUrl, headers}.
 2. App PUTs the bytes to uploadUrl (presigned, content type + length locked,
5 min).
 3. App: POST /v1/media/uploads/:id/confirm -> Confirm Heads the object and
checks size/type.
 4. A module that stores an upload id (profile photo, event banner, ...) calls
Claim, which checks owner + purpose + status and marks it attached.

Clients NEVER send keys or paths; they only ever see upload ids and signed view
URLs.


## Exported API

```go
type Service struct
func New(d *sql.DB, s storage.Storage, st *settings.Store) *Service
func (s *Service) Claim(ctx context.Context, q db.Querier, userID, uploadID int64, purpose string) error
func (s *Service) Confirm(ctx context.Context, userID, uploadID int64) (Upload, error)
func (s *Service) CreateUpload(ctx context.Context, userID int64, purpose, contentType string, size int64) (Ticket, error)
func (s *Service) OwnerOf(ctx context.Context, uploadID int64) (int64, error)
func (s *Service) ViewURL(ctx context.Context, uploadID int64) string
func (s *Service) ViewURLs(ctx context.Context, ids []int64) map[int64]string
type Ticket struct
type Upload struct
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/media`). Keep the package comment true and update this file when the API changes._
