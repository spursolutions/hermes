package sqlpool

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Options control per-tenant pool lifecycle.
//
// MaxConns / MaxIdleConns / ConnMaxLifetime configure the standard
// database/sql settings on every per-tenant *sqlx.DB. IdleTTL and
// SweepInterval drive the background janitor that evicts whole pools
// when a tenant has been silent for IdleTTL.
type Options struct {
	MaxConns        int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	IdleTTL         time.Duration
	SweepInterval   time.Duration
}

// DefaultOptions returns the defaults used when no TENANT_POOL_* env vars are set.
func DefaultOptions() Options {
	return Options{
		MaxConns:        5,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		IdleTTL:         30 * time.Minute,
		SweepInterval:   5 * time.Minute,
	}
}

// Env var names read by OptionsFromEnv.
const (
	EnvMaxConns        = "TENANT_POOL_MAX_CONNS"
	EnvMaxIdleConns    = "TENANT_POOL_MAX_IDLE_CONNS"
	EnvConnMaxLifetime = "TENANT_POOL_CONN_MAX_LIFETIME"
	EnvIdleTTL         = "TENANT_POOL_IDLE_TTL"
	EnvSweepInterval   = "TENANT_POOL_SWEEP_INTERVAL"
)

// OptionsFromEnv reads TENANT_POOL_* env vars over DefaultOptions and
// validates the result. Unset or empty vars keep their default.
func OptionsFromEnv() (Options, error) {
	return optionsFrom(os.Getenv)
}

func optionsFrom(getenv func(string) string) (Options, error) {
	o := DefaultOptions()
	var err error

	if o.MaxConns, err = intVar(getenv, EnvMaxConns, o.MaxConns); err != nil {
		return Options{}, err
	}
	if o.MaxIdleConns, err = intVar(getenv, EnvMaxIdleConns, o.MaxIdleConns); err != nil {
		return Options{}, err
	}
	if o.ConnMaxLifetime, err = durationVar(getenv, EnvConnMaxLifetime, o.ConnMaxLifetime); err != nil {
		return Options{}, err
	}
	if o.IdleTTL, err = durationVar(getenv, EnvIdleTTL, o.IdleTTL); err != nil {
		return Options{}, err
	}
	if o.SweepInterval, err = durationVar(getenv, EnvSweepInterval, o.SweepInterval); err != nil {
		return Options{}, err
	}

	return o, o.Validate()
}

// Validate reports the first invalid setting, naming its env var.
func (o Options) Validate() error {
	switch {
	case o.MaxConns < 1:
		return fmt.Errorf("invalid %s: %d must be at least 1", EnvMaxConns, o.MaxConns)
	case o.MaxIdleConns < 0:
		return fmt.Errorf("invalid %s: %d must be non-negative", EnvMaxIdleConns, o.MaxIdleConns)
	case o.MaxIdleConns > o.MaxConns:
		return fmt.Errorf("invalid %s: %d cannot exceed %s (%d)", EnvMaxIdleConns, o.MaxIdleConns, EnvMaxConns, o.MaxConns)
	case o.ConnMaxLifetime < 0:
		return fmt.Errorf("invalid %s: %s must be non-negative", EnvConnMaxLifetime, o.ConnMaxLifetime)
	case o.IdleTTL <= 0:
		return fmt.Errorf("invalid %s: %s must be greater than zero", EnvIdleTTL, o.IdleTTL)
	case o.SweepInterval <= 0:
		return fmt.Errorf("invalid %s: %s must be greater than zero", EnvSweepInterval, o.SweepInterval)
	}
	return nil
}

func intVar(getenv func(string) string, key string, def int) (int, error) {
	raw := getenv(key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}

func durationVar(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return d, nil
}
