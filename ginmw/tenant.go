// Package ginmw adapts Hermes to the Gin web framework.
package ginmw

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/spursolutions/hermes/tenancy"
)

// ContextKey is the Gin context key under which the resolved tenant ID is
// exposed for handlers and logging. Code that only sees context.Context
// should use tenancy.IDFromContext instead.
const ContextKey = "tenant_id"

// ErrorHandler writes the response for a rejected request. err is
// tenancy.ErrMissing or a tenancy.ErrUnknown match; pass it through
// tenancy.ClientMessage for a client-safe message.
type ErrorHandler func(c *gin.Context, err error)

// ResolvedHook runs after a tenant has been accepted and attached to the
// request context, e.g. to tag the active trace span.
type ResolvedHook func(c *gin.Context, tenantID string)

// Option customizes the Tenant middleware.
type Option func(*config)

type config struct {
	onError    ErrorHandler
	onResolved ResolvedHook
}

// WithErrorHandler replaces the default 400 {"error": "..."} response so a
// service can render rejections in its own error format.
func WithErrorHandler(h ErrorHandler) Option {
	return func(c *config) { c.onError = h }
}

// WithOnResolved registers a hook that runs for every accepted request.
func WithOnResolved(h ResolvedHook) Option {
	return func(c *config) { c.onResolved = h }
}

// Tenant returns middleware that establishes tenant context for every
// request. The tenant is read from the gateway-injected tenancy.HeaderName
// header (tenant_id|user_id). Requests without a tenant, or whose tenant is
// not in the catalog, are rejected before any handler runs, so downstream
// code can assume tenant context is present.
//
// On success the tenant ID and raw header are attached to the request's
// context.Context (tenancy.IDFromContext / tenancy.RawHeaderFromContext)
// and the tenant ID is set on the Gin context under ContextKey.
func Tenant(catalog *tenancy.Catalog, opts ...Option) gin.HandlerFunc {
	cfg := config{onError: defaultErrorHandler}
	for _, o := range opts {
		o(&cfg)
	}

	return func(c *gin.Context) {
		raw := c.GetHeader(tenancy.HeaderName)
		tenantID, ok := tenancy.IDFromHeader(raw)
		if !ok {
			cfg.onError(c, tenancy.ErrMissing)
			c.Abort()
			return
		}

		if !catalog.Has(tenantID) {
			cfg.onError(c, tenancy.Unknown(tenantID))
			c.Abort()
			return
		}

		c.Set(ContextKey, tenantID)
		ctx := tenancy.WithID(c.Request.Context(), tenantID)
		ctx = tenancy.WithRawHeader(ctx, raw)
		c.Request = c.Request.WithContext(ctx)

		if cfg.onResolved != nil {
			cfg.onResolved(c, tenantID)
		}

		c.Next()
	}
}

func defaultErrorHandler(c *gin.Context, err error) {
	msg, _ := tenancy.ClientMessage(err)
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}
