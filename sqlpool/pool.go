// Package sqlpool provides lazily-opened, per-tenant SQL Server connection
// pools keyed by the tenant carried on context.Context.
package sqlpool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/microsoft/go-mssqldb"

	"github.com/spursolutions/hermes/tenancy"
)

// DBProvider resolves the tenant-specific *sqlx.DB for a request.
// Repositories depend on this interface rather than on a fixed *sqlx.DB
// so that data access follows the tenant context of each request.
type DBProvider interface {
	DB(ctx context.Context) (*sqlx.DB, error)
}

// Opener creates a *sqlx.DB for a given connection string. It is plumbed
// through the Pool so that tests can substitute a fake driver.
type Opener func(connectionString string) (*sqlx.DB, error)

// DefaultOpener connects with the Microsoft SQL Server driver.
func DefaultOpener(connectionString string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("sqlserver", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to tenant database: %w", err)
	}
	return db, nil
}

// Pool maintains a lazily-populated registry of per-tenant SQL connection
// pools. Each pool shares the same base connection settings; only the
// database attribute differs per tenant.
//
// Each entry tracks its last-used timestamp. A background janitor
// (StartJanitor) evicts entries idle for longer than IdleTTL and closes
// their *sqlx.DB. The next request for an evicted tenant reopens its pool.
type Pool struct {
	catalog *tenancy.Catalog
	base    BaseConnString
	opener  Opener
	opts    Options

	now func() time.Time

	mu      sync.Mutex
	entries map[string]*poolEntry
}

type poolEntry struct {
	db       *sqlx.DB
	lastUsed time.Time
}

var _ DBProvider = (*Pool)(nil)

// New constructs a Pool that resolves tenant DBs from the given catalog and
// shared base connection string. The returned Pool is safe for concurrent
// use. Call StartJanitor to enable background eviction of idle pools.
func New(catalog *tenancy.Catalog, base BaseConnString, opts Options) *Pool {
	return &Pool{
		catalog: catalog,
		base:    base,
		opener:  DefaultOpener,
		opts:    opts,
		now:     time.Now,
		entries: make(map[string]*poolEntry),
	}
}

// WithOpener overrides the connection opener (primarily for tests).
func (p *Pool) WithOpener(o Opener) *Pool {
	p.opener = o
	return p
}

// withClock overrides the time source (test-only).
func (p *Pool) withClock(now func() time.Time) *Pool {
	p.now = now
	return p
}

// DB returns the *sqlx.DB for the tenant carried on ctx, opening it on
// demand. It returns tenancy.ErrMissing when ctx has no tenant and a
// tenancy.ErrUnknown match when the tenant is not in the catalog. Every
// successful call resets the tenant's idle timer.
func (p *Pool) DB(ctx context.Context) (*sqlx.DB, error) {
	tenantID, ok := tenancy.IDFromContext(ctx)
	if !ok {
		return nil, tenancy.ErrMissing
	}
	dbName, ok := p.catalog.DatabaseFor(tenantID)
	if !ok {
		return nil, tenancy.Unknown(tenantID)
	}

	key := tenancy.Normalize(tenantID)

	p.mu.Lock()
	defer p.mu.Unlock()

	if e, exists := p.entries[key]; exists {
		e.lastUsed = p.now()
		return e.db, nil
	}

	db, err := p.opener(p.base.ForDatabase(dbName))
	if err != nil {
		return nil, fmt.Errorf("failed to open pool for tenant %s: %w", tenantID, err)
	}

	db.SetMaxOpenConns(p.opts.MaxConns)
	db.SetMaxIdleConns(p.opts.MaxIdleConns)
	db.SetConnMaxLifetime(p.opts.ConnMaxLifetime)

	p.entries[key] = &poolEntry{db: db, lastUsed: p.now()}
	slog.InfoContext(ctx, "tenant pool opened",
		"tenant_id", tenantID,
		"max_conns", p.opts.MaxConns,
		"max_idle_conns", p.opts.MaxIdleConns,
		"conn_max_lifetime", p.opts.ConnMaxLifetime,
	)
	return db, nil
}

// StartJanitor launches a background goroutine that periodically evicts
// pools idle for longer than IdleTTL. The goroutine exits when ctx is
// cancelled. It is a no-op when IdleTTL or SweepInterval is not positive.
func (p *Pool) StartJanitor(ctx context.Context) {
	if p.opts.SweepInterval <= 0 || p.opts.IdleTTL <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(p.opts.SweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.sweep()
			}
		}
	}()
}

// sweep removes pool entries idle longer than IdleTTL and closes them
// outside the registry lock so slow Close calls never block lookups.
func (p *Pool) sweep() {
	cutoff := p.now().Add(-p.opts.IdleTTL)

	type stale struct {
		id string
		db *sqlx.DB
	}
	var evicted []stale

	p.mu.Lock()
	for id, e := range p.entries {
		if e.lastUsed.Before(cutoff) {
			evicted = append(evicted, stale{id: id, db: e.db})
			delete(p.entries, id)
		}
	}
	p.mu.Unlock()

	for _, s := range evicted {
		if err := s.db.Close(); err != nil {
			slog.Warn("failed to close idle tenant pool", "tenant_id", s.id, "error", err)
			continue
		}
		slog.Info("evicted idle tenant pool", "tenant_id", s.id)
	}
}

// Close releases every open per-tenant pool. It returns the first error
// encountered but always attempts to close the rest.
func (p *Pool) Close() error {
	p.mu.Lock()
	entries := p.entries
	p.entries = make(map[string]*poolEntry)
	p.mu.Unlock()

	var firstErr error
	for id, e := range entries {
		if err := e.db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close pool for tenant %s: %w", id, err)
		}
	}
	return firstErr
}

// Tenants returns the tenant IDs that currently have an open pool.
func (p *Pool) Tenants() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.entries))
	for id := range p.entries {
		out = append(out, id)
	}
	return out
}

// Static returns a DBProvider that always yields the same *sqlx.DB. It is
// intended for tests and any non-tenant context that still needs to
// satisfy the DBProvider contract.
func Static(db *sqlx.DB) DBProvider {
	return staticProvider{db: db}
}

type staticProvider struct{ db *sqlx.DB }

func (s staticProvider) DB(context.Context) (*sqlx.DB, error) { return s.db, nil }
