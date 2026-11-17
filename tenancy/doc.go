// Package tenancy is the dependency-free core of Hermes: the tenant
// catalog, tenant identity on context.Context, header parsing, and the
// routing errors every tenant-aware component returns.
//
// It deliberately knows nothing about HTTP frameworks or databases.
// Framework adapters live in ginmw; per-tenant SQL pools live in sqlpool.
package tenancy
