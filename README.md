# Hermes

Shared multi-tenancy for Go services: one shared app, one SQL Server database per tenant.

| Package | What it does | Depends on |
|---|---|---|
| `tenancy` | Tenant catalog, tenant ID on `context.Context`, header parsing, routing errors | stdlib only |
| `sqlpool` | Lazily-opened per-tenant `*sqlx.DB` pools with idle eviction; `TENANT_POOL_*` config | `tenancy`, sqlx, mssql |
| `ginmw` | Gin middleware that resolves and validates the tenant on every request | `tenancy`, gin |

## Installing

The repo is private, so tell Go not to use the public proxy/checksum DB for it:

```sh
go env -w GOPRIVATE=github.com/spursolutions/*
go get github.com/spursolutions/hermes@latest
```

## Plugging it into a service

```go
catalog, err := tenancy.LoadCatalogFromEnv()   // tenant_<id>=<database> env vars
poolOpts, err := sqlpool.OptionsFromEnv()      // TENANT_POOL_* with defaults + validation

pool := sqlpool.New(catalog, sqlpool.BaseConnString(sharedConnStr), poolOpts)
pool.StartJanitor(ctx)
defer pool.Close()

api.Use(ginmw.Tenant(catalog, ginmw.WithErrorHandler(apperror.Respond)))
```

Repositories take a `sqlpool.DBProvider` and call `db.DB(ctx)` per query. In tests use
`sqlpool.Static(db)` and `tenancy.WithID(ctx, "acme")`.

## Errors

`sqlpool.Pool.DB` and `ginmw.Tenant` return `tenancy.ErrMissing` or a `tenancy.ErrUnknown`
match. They carry no HTTP status; map them in your service's error responder:

```go
if msg, ok := tenancy.ClientMessage(err); ok {
    c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})
    return
}
```

`ClientMessage` sees through `fmt.Errorf("...: %w", err)` wrapping, so errors bubbling up
from repositories still render as 400s.

## Configuration

| Env var | Default | Meaning |
|---|---|---|
| `tenant_<id>` | — | Tenant ID → database name. ID: lowercase alphanumeric, `_`, `-` |
| `TENANT_POOL_MAX_CONNS` | `5` | Max open connections per tenant |
| `TENANT_POOL_MAX_IDLE_CONNS` | `5` | Max idle connections per tenant (≤ max conns) |
| `TENANT_POOL_CONN_MAX_LIFETIME` | `5m` | Recycle individual connections after this long |
| `TENANT_POOL_IDLE_TTL` | `30m` | Close a tenant's whole pool after this much inactivity |
| `TENANT_POOL_SWEEP_INTERVAL` | `5m` | How often idle pools are checked |
