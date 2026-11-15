# `internal/platform/authn`

Package authn issues/verifies access tokens and provides the auth middleware
that EVERY route group uses (see internal/app/router.go). Refresh tokens,
OTP and admin login live in modules/auth and modules/admin; this package only
handles stateless access tokens.


## Exported API

```go
func AdminAuth(iss *Issuer, admins AdminChecker) gin.HandlerFunc
func AdminID(c *gin.Context) int64
func HasPermission(c *gin.Context, perm string) bool
func RequireOnboarded() gin.HandlerFunc
func RequirePermission(perm string) gin.HandlerFunc
func SetTestAdmin(c *gin.Context, adminID int64, perms []string)
func SetTestUser(c *gin.Context, u userdir.Basic)
func User(c *gin.Context) userdir.Basic
func UserAuth(iss *Issuer, users *userdir.Directory) gin.HandlerFunc
func UserID(c *gin.Context) int64
type AdminChecker interface
type Claims struct
func (c *Claims) SubjectID() int64
type Issuer struct
func NewIssuer(k SigningKeys, issuer string) (*Issuer, error)
func (i *Issuer) IssueAdmin(adminID, roleID int64, perms []string, ttl time.Duration) (string, time.Time, error)
func (i *Issuer) IssueUser(userID int64, ttl time.Duration) (string, time.Time, error)
func (i *Issuer) ParseAdmin(tok string) (*Claims, error)
func (i *Issuer) ParseUser(tok string) (*Claims, error)
type SigningKeys struct
```

_Generated from the Go doc comments (`go doc -all ./internal/platform/authn`). Keep the package comment true and update this file when the API changes._
