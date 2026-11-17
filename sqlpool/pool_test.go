package sqlpool

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/spursolutions/hermes/tenancy"
)

func testOpts() Options {
	return Options{MaxConns: 5, MaxIdleConns: 5, ConnMaxLifetime: 5 * time.Minute, IdleTTL: 30 * time.Minute, SweepInterval: 5 * time.Minute}
}

func newMockDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	mock.ExpectClose()
	return sqlx.NewDb(db, "sqlserver")
}

func TestPool_DB_RoutesByTenant(t *testing.T) {
	catalog := tenancy.NewCatalog(map[string]string{
		"acme":   "AcmeDB",
		"globex": "GlobexDB",
	})

	var (
		mu    sync.Mutex
		opens []string
	)

	p := New(catalog, BaseConnString("server=sql.test;database=Placeholder;user id=sa"), testOpts()).
		WithOpener(func(connStr string) (*sqlx.DB, error) {
			mu.Lock()
			defer mu.Unlock()
			opens = append(opens, connStr)
			return newMockDB(t), nil
		})
	t.Cleanup(func() { _ = p.Close() })

	ctxAcme := tenancy.WithID(context.Background(), "acme")
	ctxGlobex := tenancy.WithID(context.Background(), "globex")

	dbA1, err := p.DB(ctxAcme)
	if err != nil {
		t.Fatalf("DB(acme) error = %v", err)
	}
	dbA2, err := p.DB(ctxAcme)
	if err != nil {
		t.Fatalf("DB(acme second) error = %v", err)
	}
	if dbA1 != dbA2 {
		t.Fatal("same tenant must reuse pool")
	}

	dbG, err := p.DB(ctxGlobex)
	if err != nil {
		t.Fatalf("DB(globex) error = %v", err)
	}
	if dbA1 == dbG {
		t.Fatal("different tenants must have distinct pools")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(opens) != 2 {
		t.Fatalf("opens = %d, want 2", len(opens))
	}
}

func TestPool_DB_MissingTenant(t *testing.T) {
	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts())
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.DB(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, tenancy.ErrMissing) {
		t.Fatalf("expected ErrMissing, got %v", err)
	}
}

func TestPool_DB_UnknownTenant(t *testing.T) {
	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts())
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.DB(tenancy.WithID(context.Background(), "rogue"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, tenancy.ErrUnknown) {
		t.Fatalf("expected ErrUnknown, got %v", err)
	}
}

func TestPool_DB_OpenerError(t *testing.T) {
	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts()).
		WithOpener(func(string) (*sqlx.DB, error) { return nil, errors.New("boom") })

	_, err := p.DB(tenancy.WithID(context.Background(), "acme"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPool_Static(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	mock.ExpectClose()
	defer func() { _ = db.Close() }()
	xdb := sqlx.NewDb(db, "sqlserver")

	provider := Static(xdb)
	got, err := provider.DB(context.Background())
	if err != nil {
		t.Fatalf("Static().DB() error = %v", err)
	}
	if got != xdb {
		t.Fatal("expected same DB instance")
	}
}

func TestPool_LazyOpenOnce(t *testing.T) {
	var opens int32
	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts()).
		WithOpener(func(string) (*sqlx.DB, error) {
			atomic.AddInt32(&opens, 1)
			return newMockDB(t), nil
		})
	t.Cleanup(func() { _ = p.Close() })

	ctx := tenancy.WithID(context.Background(), "acme")
	for range 5 {
		if _, err := p.DB(ctx); err != nil {
			t.Fatalf("DB() error = %v", err)
		}
	}
	if atomic.LoadInt32(&opens) != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestPool_Sweep_EvictsIdleTenant(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	var opens int32
	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts()).
		WithOpener(func(string) (*sqlx.DB, error) {
			atomic.AddInt32(&opens, 1)
			return newMockDB(t), nil
		}).
		withClock(func() time.Time { return now })
	t.Cleanup(func() { _ = p.Close() })

	ctx := tenancy.WithID(context.Background(), "acme")
	first, err := p.DB(ctx)
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if len(p.Tenants()) != 1 {
		t.Fatalf("tenants = %v, want 1 tenant", p.Tenants())
	}

	now = now.Add(31 * time.Minute)
	p.sweep()
	if len(p.Tenants()) != 0 {
		t.Fatalf("expected idle pool to be evicted, got %v", p.Tenants())
	}

	second, err := p.DB(ctx)
	if err != nil {
		t.Fatalf("DB() second error = %v", err)
	}
	if first == second {
		t.Fatal("expected new pool after eviction")
	}
	if atomic.LoadInt32(&opens) != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestPool_DB_ResetsIdleTimer(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	p := New(tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}), "server=sql.test", testOpts()).
		WithOpener(func(string) (*sqlx.DB, error) { return newMockDB(t), nil }).
		withClock(func() time.Time { return now })
	t.Cleanup(func() { _ = p.Close() })

	ctx := tenancy.WithID(context.Background(), "acme")
	first, err := p.DB(ctx)
	if err != nil {
		t.Fatalf("DB() error = %v", err)
	}

	now = now.Add(29 * time.Minute)
	second, err := p.DB(ctx)
	if err != nil {
		t.Fatalf("DB() second error = %v", err)
	}
	if first != second {
		t.Fatal("expected same pool to be reused")
	}

	now = now.Add(29 * time.Minute)
	p.sweep()
	if len(p.Tenants()) != 1 {
		t.Fatalf("pool should survive, got tenants=%v", p.Tenants())
	}
}

func TestPool_StartJanitor_RunsAndStops(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	var offsetNs atomic.Int64
	clock := func() time.Time { return base.Add(time.Duration(offsetNs.Load())) }

	p := New(
		tenancy.NewCatalog(map[string]string{"acme": "AcmeDB"}),
		"server=sql.test",
		Options{MaxConns: 5, MaxIdleConns: 5, ConnMaxLifetime: time.Second, IdleTTL: 50 * time.Millisecond, SweepInterval: 10 * time.Millisecond},
	).
		WithOpener(func(string) (*sqlx.DB, error) { return newMockDB(t), nil }).
		withClock(clock)
	t.Cleanup(func() { _ = p.Close() })

	ctx := tenancy.WithID(context.Background(), "acme")
	if _, err := p.DB(ctx); err != nil {
		t.Fatalf("DB() error = %v", err)
	}
	if len(p.Tenants()) != 1 {
		t.Fatalf("tenants = %v, want 1 tenant", p.Tenants())
	}

	janitorCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p.StartJanitor(janitorCtx)

	offsetNs.Store(int64(time.Second))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(p.Tenants()) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("janitor should evict idle pool")
}
