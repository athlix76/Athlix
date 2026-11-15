# `internal/platform/userdir`

Package userdir is the shared, read-only view of users that every module may
use: auth status checks, public user summaries, follow and block lookups.

Writes to users/profiles/follows/blocks happen ONLY in their owning modules
(auth, user, profile, follow). After a write that changes Basic fields, call
Invalidate.


## Exported API

```go
type Basic struct
func (b Basic) HasRole(role string) bool
type Directory struct
func New(d *sql.DB, photos PhotoSigner) *Directory
func (d *Directory) AdminActive(ctx context.Context, adminID int64) (bool, error)
func (d *Directory) BlockedEitherWay(ctx context.Context, a, b int64) (bool, error)
func (d *Directory) Follows(ctx context.Context, follower, followee int64) (bool, error)
func (d *Directory) Get(ctx context.Context, id int64) (Basic, error)
func (d *Directory) Invalidate(id int64)
func (d *Directory) MutualFollow(ctx context.Context, a, b int64) (bool, error)
func (d *Directory) SetPhotoSigner(p PhotoSigner)
func (d *Directory) Summaries(ctx context.Context, ids []int64) (map[int64]Summary, error)
func (d *Directory) Summary(ctx context.Context, id int64) (Summary, bool, error)
type PhotoSigner interface
type SportRef struct
type Summary struct
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/userdir`). Keep the package comment true and update this file when the API changes._
