package authn

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
	"athlix/backend/internal/platform/userdir"
)

const (
	ctxUserID     = "authn.userId"
	ctxUser       = "authn.user"
	ctxAdminID    = "authn.adminId"
	ctxAdminPerms = "authn.adminPerms"
)

// AdminChecker reports whether an admin account is still active.
type AdminChecker interface {
	AdminActive(ctx context.Context, adminID int64) (bool, error)
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// UserAuth authenticates an app user from the Bearer access token and loads their current
// status (suspension takes effect within the userdir cache TTL, ~10s).
func UserAuth(iss *Issuer, users *userdir.Directory) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := iss.ParseUser(bearer(c))
		if err != nil {
			httpx.Fail(c, httpx.Unauthenticated())
			return
		}
		u, err := users.Get(c.Request.Context(), claims.SubjectID())
		if err != nil || u.Status == "deleted" {
			httpx.Fail(c, httpx.Unauthenticated())
			return
		}
		if u.Status == "suspended" {
			httpx.Fail(c, httpx.Coded(http.StatusForbidden, httpx.CodeSuspended, "Your account is suspended. Contact support."))
			return
		}
		c.Set(ctxUserID, u.ID)
		c.Set(ctxUser, u)
		ctx := logger.With(c.Request.Context(), logger.From(c.Request.Context()).With(slog.Int64("userId", u.ID)))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequireOnboarded blocks users who have not finished onboarding (role, DigiLocker
// verification, guardian consent for minors, profile). Must run after UserAuth.
func RequireOnboarded() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := User(c)
		if u.OnboardingStep != "done" {
			e := httpx.Coded(http.StatusForbidden, httpx.CodeOnboarding, "Please finish setting up your account.")
			e.Fields = map[string]string{"nextStep": u.OnboardingStep}
			httpx.Fail(c, e)
			return
		}
		c.Next()
	}
}

// AdminAuth authenticates an admin-panel user from the Bearer admin access token.
func AdminAuth(iss *Issuer, admins AdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := iss.ParseAdmin(bearer(c))
		if err != nil {
			httpx.Fail(c, httpx.Unauthenticated())
			return
		}
		ok, err := admins.AdminActive(c.Request.Context(), claims.SubjectID())
		if err != nil || !ok {
			httpx.Fail(c, httpx.Unauthenticated())
			return
		}
		c.Set(ctxAdminID, claims.SubjectID())
		c.Set(ctxAdminPerms, claims.Permissions)
		ctx := logger.With(c.Request.Context(), logger.From(c.Request.Context()).With(slog.Int64("adminId", claims.SubjectID())))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequirePermission allows the request only if the admin has perm (or "*").
// Permission strings are a closed list in docs/security.md.
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasPermission(c, perm) {
			httpx.Fail(c, httpx.Forbidden("You do not have permission for this action."))
			return
		}
		c.Next()
	}
}

// UserID is the authenticated app user's id (from the verified token ONLY). Never read a
// "who is calling" id from the body, query, path or headers.
func UserID(c *gin.Context) int64 { return c.GetInt64(ctxUserID) }

// User is the authenticated app user's cached directory record.
func User(c *gin.Context) userdir.Basic {
	v, _ := c.Get(ctxUser)
	u, _ := v.(userdir.Basic)
	return u
}

// AdminID is the authenticated admin's id.
func AdminID(c *gin.Context) int64 { return c.GetInt64(ctxAdminID) }

// HasPermission reports whether the authenticated admin holds perm.
func HasPermission(c *gin.Context, perm string) bool {
	v, _ := c.Get(ctxAdminPerms)
	perms, _ := v.([]string)
	return slices.Contains(perms, "*") || slices.Contains(perms, perm)
}

// SetTestUser is for handler tests: marks the request as authenticated as u.
func SetTestUser(c *gin.Context, u userdir.Basic) { c.Set(ctxUserID, u.ID); c.Set(ctxUser, u) }

// SetTestAdmin is for handler tests.
func SetTestAdmin(c *gin.Context, adminID int64, perms []string) {
	c.Set(ctxAdminID, adminID)
	c.Set(ctxAdminPerms, perms)
}
