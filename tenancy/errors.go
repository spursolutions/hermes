package tenancy

import "errors"

// ErrMissing indicates that no tenant identity was present on the request.
// It is a routing-level failure, not a domain not-found result.
var ErrMissing = errors.New("tenant identity is required")

// ErrUnknown indicates that the tenant identity on the request does not
// resolve to any entry in the tenant catalog. Errors returned by this
// package for an unknown tenant are *UnknownTenantError and match
// ErrUnknown via errors.Is.
var ErrUnknown = errors.New("unknown tenant")

// UnknownTenantError names the tenant that failed catalog lookup.
type UnknownTenantError struct {
	TenantID string
}

func (e *UnknownTenantError) Error() string { return "unknown tenant: " + e.TenantID }

// Is makes errors.Is(err, ErrUnknown) true for any *UnknownTenantError.
func (e *UnknownTenantError) Is(target error) bool { return target == ErrUnknown }

// Unknown returns the error for a tenant that is not in the catalog.
func Unknown(tenantID string) error {
	return &UnknownTenantError{TenantID: tenantID}
}

// ClientMessage reports whether err is a tenant routing error and, if so,
// returns the message that is safe to send to the client. Services use it
// to map tenant failures onto their own error responses (typically 400),
// even when the error has been wrapped on its way up from a repository.
func ClientMessage(err error) (string, bool) {
	var unknown *UnknownTenantError
	if errors.As(err, &unknown) {
		return unknown.Error(), true
	}
	if errors.Is(err, ErrMissing) {
		return ErrMissing.Error(), true
	}
	return "", false
}
