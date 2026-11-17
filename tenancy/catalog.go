package tenancy

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Catalog is the in-memory view of the tenant routing metadata. It maps
// the canonical tenant identity (the gateway-injected tenant ID) to the
// tenant database name.
type Catalog struct {
	mu  sync.RWMutex
	dbs map[string]string
}

// NewCatalog returns a Catalog populated from the given tenant ID ->
// database name mapping.
func NewCatalog(mapping map[string]string) *Catalog {
	c := &Catalog{dbs: make(map[string]string, len(mapping))}
	for tenantID, dbName := range mapping {
		c.dbs[Normalize(tenantID)] = dbName
	}
	return c
}

// DatabaseFor returns the tenant database name for the given tenant ID,
// or false if the tenant is not registered.
func (c *Catalog) DatabaseFor(tenantID string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	name, ok := c.dbs[Normalize(tenantID)]
	return name, ok
}

// Has reports whether a tenant ID is registered in the catalog.
func (c *Catalog) Has(tenantID string) bool {
	_, ok := c.DatabaseFor(tenantID)
	return ok
}

// IDs returns the registered tenant IDs in arbitrary order. Useful for
// startup logging and operational visibility.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.dbs))
	for id := range c.dbs {
		out = append(out, id)
	}
	return out
}

// Size reports how many tenants are registered.
func (c *Catalog) Size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.dbs)
}

// EnvPrefix is the required prefix on every tenant catalog env var.
// Catalog entries are injected from a ConfigMap as "tenant_<tenantID>" so
// they can never collide with unrelated process/system env vars.
const EnvPrefix = "tenant_"

// LoadCatalogFromEnv loads the tenant catalog from environment variables,
// typically injected from a Kubernetes ConfigMap via env/envFrom. Each
// tenant entry is named "tenant_<tenantID>" and its value is the tenant
// database name. An empty catalog is an error: a service with no tenants
// would reject every request.
func LoadCatalogFromEnv() (*Catalog, error) {
	if c := loadFromEnv(os.Environ()); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("tenant catalog is empty: no %s* mappings found in environment", EnvPrefix)
}

func loadFromEnv(environ []string) *Catalog {
	mapping := make(map[string]string)
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		tenantID, ok := tenantIDFromEnvKey(key)
		if !ok {
			// A malformed tenant_ key is a deployment mistake, not an
			// unrelated env var: dropped silently, every request 400s.
			if strings.HasPrefix(key, EnvPrefix) {
				slog.Warn("ignoring malformed tenant catalog env var", "env_key", key)
			}
			continue
		}

		dbName := strings.TrimSpace(value)
		if dbName == "" {
			slog.Warn("ignoring tenant catalog env var with empty database name", "env_key", key)
			continue
		}

		mapping[tenantID] = dbName
	}

	if len(mapping) == 0 {
		return nil
	}

	return NewCatalog(mapping)
}

// tenantIDFromEnvKey extracts the tenant ID from a tenant catalog env var key.
// Valid keys are "tenant_<tenantID>" where the tenant ID is lowercase
// alphanumeric/underscore/hyphen; it returns the tenant ID (without the prefix)
// and true, or "", false for any non-tenant env var. Hyphens are allowed
// because tenant IDs double as DNS subdomains, where "-" is legal.
func tenantIDFromEnvKey(key string) (string, bool) {
	tenantID, ok := strings.CutPrefix(key, EnvPrefix)
	if !ok || tenantID == "" {
		return "", false
	}

	for _, r := range tenantID {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' || r == '-' {
			continue
		}
		return "", false
	}

	return tenantID, true
}
