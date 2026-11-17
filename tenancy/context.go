package tenancy

import (
	"context"
	"strings"
)

// HeaderName is the gateway-injected request header that carries the
// canonical tenant identity. Its value is expected to be tenant_id|user_id.
const HeaderName = "X-Authenticated-Userid"

type idKey struct{}
type rawHeaderKey struct{}

// WithID returns a copy of ctx that carries the given tenant ID.
func WithID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, idKey{}, tenantID)
}

// IDFromContext returns the tenant ID stored in ctx and a flag indicating
// whether one was present. Callers that require tenant context should
// treat a missing tenant as a routing failure (ErrMissing), not a domain miss.
func IDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	id, ok := ctx.Value(idKey{}).(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// WithRawHeader returns a copy of ctx that carries the original tenant
// header value, so outbound calls can forward it unchanged.
func WithRawHeader(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, rawHeaderKey{}, raw)
}

// RawHeaderFromContext returns the original gateway-provided tenant header value.
func RawHeaderFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	raw, ok := ctx.Value(rawHeaderKey{}).(string)
	if !ok || raw == "" {
		return "", false
	}
	return raw, true
}

// IDFromHeader extracts the tenant ID from the gateway-provided
// authentication header value. The expected format is tenant_id|user_id.
func IDFromHeader(value string) (string, bool) {
	tenantID, _, _ := strings.Cut(strings.TrimSpace(value), "|")
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", false
	}
	return tenantID, true
}

// Normalize returns the canonical lookup key for a tenant ID, used by every
// per-tenant registry so a tenant resolves to the same entry regardless of
// surrounding whitespace or letter case.
func Normalize(tenantID string) string {
	return strings.ToLower(strings.TrimSpace(tenantID))
}
