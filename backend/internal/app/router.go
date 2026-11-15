package app

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"athlix/backend/internal/platform/authn"
	"athlix/backend/internal/platform/httpx"
	"athlix/backend/internal/platform/logger"
)

// Routes are the ONLY places a module may register handlers. Pick the group by caller:
//
//	Public      /v1          no login. Only: health, bootstrap config, policies, sports list,
//	                         OTP request/verify, token refresh, DigiLocker OAuth callback
//	                         (bound by one-time state). Must never return user data.
//	User        /v1          logged-in app user, onboarding NOT required (onboarding endpoints, /me)
//	Verified    /v1          logged-in AND onboarding done (DigiLocker verified, guardian consent
//	                         if minor, profile created). Almost every feature goes here.
//	AdminPublic /v1/admin    admin login only
//	Admin       /v1/admin    admin token; ALSO add authn.RequirePermission("<perm>") per route
//	Webhooks    /v1/webhooks payment / store callbacks; handler MUST verify the provider signature
//	                         before reading anything else
type Routes struct {
	Public      *gin.RouterGroup
	User        *gin.RouterGroup
	Verified    *gin.RouterGroup
	AdminPublic *gin.RouterGroup
	Admin       *gin.RouterGroup
	Webhooks    *gin.RouterGroup
	Engine      *gin.Engine // only for ENV=local dev helpers (e.g. local WebSocket)
}

// NewRouter builds the gin engine with common middleware and mounts every module.
func NewRouter(d *Deps, modules []Module) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 1 << 20
	r.Use(requestContext(d), recoverer(), cors(d), bodyLimit(1<<20))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.NotFound("Route")) })

	v1 := r.Group("/v1")
	v1.GET("/health", func(c *gin.Context) { httpx.OK(c, gin.H{"status": "ok"}) })

	userGroup := v1.Group("", authn.UserAuth(d.Tokens, d.Users), maintenance(d))
	rt := Routes{
		Public:      v1.Group(""),
		User:        userGroup,
		Verified:    userGroup.Group("", authn.RequireOnboarded()),
		AdminPublic: v1.Group("/admin"),
		Admin:       v1.Group("/admin", authn.AdminAuth(d.Tokens, d.Users)),
		Webhooks:    v1.Group("/webhooks"),
		Engine:      r,
	}
	if d.Cfg.IsLocal() && LocalStorage != nil {
		LocalStorage.RegisterRoutes(r)
	}
	for _, m := range modules {
		m.Routes(rt)
	}
	return r
}

// RegisterEventHandlers subscribes every module's handlers on d.Dispatcher.
func RegisterEventHandlers(d *Deps, modules []Module) {
	for _, m := range modules {
		if s, ok := m.(EventSubscriber); ok {
			for typ, h := range s.EventHandlers() {
				d.Dispatcher.On(typ, h)
			}
		}
	}
}

// CollectJobs returns every module's scheduled jobs by name.
func CollectJobs(modules []Module) map[string]JobFunc {
	out := map[string]JobFunc{}
	for _, m := range modules {
		if p, ok := m.(JobProvider); ok {
			for name, j := range p.Jobs() {
				out[name] = j
			}
		}
	}
	return out
}

// requestContext adds a request id and a request-scoped logger, and writes ONE access log
// line per request: method, route template, status, latency. Never the query string,
// headers or body (they can hold tokens and PII).
func requestContext(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-Id")
		if _, err := uuid.Parse(rid); err != nil {
			rid = uuid.NewString()
		}
		c.Header("X-Request-Id", rid)
		l := d.Log.With(slog.String("requestId", rid))
		c.Request = c.Request.WithContext(logger.With(c.Request.Context(), l))
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		logger.From(c.Request.Context()).Info("http",
			slog.String("method", c.Request.Method), slog.String("route", route),
			slog.Int("status", c.Writer.Status()), slog.Int64("ms", time.Since(start).Milliseconds()))
	}
}

func recoverer() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if v := recover(); v != nil {
				logger.From(c.Request.Context()).Error("panic", slog.Any("panic", v))
				httpx.Fail(c, httpx.Internal())
			}
		}()
		c.Next()
	}
}

// cors allows the admin web panel origins only (the mobile app does not need CORS).
func cors(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && slices.Contains(d.Cfg.AdminOrigins, origin) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			h.Set("Access-Control-Expose-Headers", "X-Request-Id") // the admin panel shows it on errors
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func bodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/dev/storage/") { // local uploads; own 50MB cap
			c.Next()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

// maintenance returns 503 MAINTENANCE for app-user routes while app.maintenance_mode is true.
func maintenance(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d.Settings.Bool(c.Request.Context(), "app.maintenance_mode", false) {
			httpx.Fail(c, httpx.Coded(http.StatusServiceUnavailable, httpx.CodeMaintenance, "Athlix is under maintenance. Please try again soon."))
			return
		}
		c.Next()
	}
}
