package tenancy

import (
	"bytes"
	"log/slog"
	"sort"
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	catalog := NewCatalog(map[string]string{"acme": "db_acme"})

	dbName, ok := catalog.DatabaseFor("ACME")
	if !ok || dbName != "db_acme" {
		t.Fatalf("DatabaseFor() = %q, %v; want db_acme, true", dbName, ok)
	}
	if !catalog.Has("acme") {
		t.Fatal("expected tenant to exist")
	}
	if catalog.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", catalog.Size())
	}
}

func TestCatalog_IDs(t *testing.T) {
	catalog := NewCatalog(map[string]string{"Acme": "db_acme", "globex": "db_globex"})
	ids := catalog.IDs()
	sort.Strings(ids)
	if strings.Join(ids, ",") != "acme,globex" {
		t.Fatalf("IDs() = %v, want normalized [acme globex]", ids)
	}
}

func TestCatalog_NilSafe(t *testing.T) {
	var c *Catalog
	if c.Has("acme") || c.Size() != 0 || c.IDs() != nil {
		t.Fatal("nil catalog must behave as empty")
	}
}

func TestLoadFromEnv_PrefixedKeys(t *testing.T) {
	c := loadFromEnv([]string{
		"tenant_acme=db_acme",
		"tenant_globex=db_globex",
		"PATH=/usr/bin",
		"DB_SERVER=ignored",
		"acme=ShouldBeIgnored", // unprefixed: not a tenant
	})
	if c == nil {
		t.Fatal("loadFromEnv() = nil; want catalog")
	}
	if c.Size() != 2 {
		t.Fatalf("Size() = %d, want 2", c.Size())
	}
	if dbName, ok := c.DatabaseFor("acme"); !ok || dbName != "db_acme" {
		t.Fatalf("DatabaseFor(acme) = %q, %v; want db_acme, true", dbName, ok)
	}
	if dbName, ok := c.DatabaseFor("globex"); !ok || dbName != "db_globex" {
		t.Fatalf("DatabaseFor(globex) = %q, %v; want db_globex, true", dbName, ok)
	}
}

func TestLoadFromEnv_NoTenantKeys(t *testing.T) {
	if c := loadFromEnv([]string{"PATH=/usr/bin", "client_domain=ignored", "acme=db_acme"}); c != nil {
		t.Fatalf("loadFromEnv() = catalog of size %d; want nil", c.Size())
	}
}

func TestLoadCatalogFromEnv(t *testing.T) {
	t.Setenv("tenant_acme", "db_acme")
	c, err := LoadCatalogFromEnv()
	if err != nil {
		t.Fatalf("LoadCatalogFromEnv() error = %v", err)
	}
	if !c.Has("acme") {
		t.Fatal("expected acme to be loaded from the environment")
	}
}

func TestTenantIDFromEnvKey(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		wantID string
		wantOK bool
	}{
		{name: "lowercase alphanumeric", key: "tenant_acme2", wantID: "acme2", wantOK: true},
		{name: "underscore", key: "tenant_acme_corp", wantID: "acme_corp", wantOK: true},
		{name: "hyphen", key: "tenant_acme-corp", wantID: "acme-corp", wantOK: true},
		{name: "leading hyphen", key: "tenant_-acme", wantID: "-acme", wantOK: true},
		{name: "missing prefix", key: "acme", wantID: "", wantOK: false},
		{name: "prefix only", key: "tenant_", wantID: "", wantOK: false},
		{name: "uppercase rejected", key: "tenant_Acme-Corp", wantID: "", wantOK: false},
		{name: "dot rejected", key: "tenant_acme.corp", wantID: "", wantOK: false},
		{name: "space rejected", key: "tenant_acme corp", wantID: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := tenantIDFromEnvKey(tt.key)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Fatalf("tenantIDFromEnvKey(%q) = %q, %v; want %q, %v",
					tt.key, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestLoadFromEnv_HyphenatedTenantID(t *testing.T) {
	c := loadFromEnv([]string{
		"tenant_acme-corp=AcmeCorp_Data",
		"tenant_globex=Globex_Data",
	})
	if c == nil || c.Size() != 2 {
		t.Fatalf("loadFromEnv() = %v; want catalog of size 2", c)
	}
	if dbName, ok := c.DatabaseFor("acme-corp"); !ok || dbName != "AcmeCorp_Data" {
		t.Fatalf("DatabaseFor(acme-corp) = %q, %v; want AcmeCorp_Data, true", dbName, ok)
	}

	// The header-parsed value must resolve through the same entry.
	tenantID, ok := IDFromHeader("ACME-CORP|user1")
	if !ok {
		t.Fatal("IDFromHeader() = _, false; want true")
	}
	if !c.Has(tenantID) {
		t.Fatalf("Has(%q) = false; want true", tenantID)
	}
}

func TestLoadFromEnv_WarnsOnDroppedTenantKeys(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	c := loadFromEnv([]string{
		"tenant_acme=db_acme",
		"tenant_Bad.Key=db_bad", // malformed: uppercase and dot
		"tenant_novalue=   ",    // prefixed but no database name
		"PATH=/usr/bin",         // unprefixed: not a tenant, must stay quiet
		"client_domain=ignored", // unprefixed: not a tenant, must stay quiet
	})
	if c == nil || c.Size() != 1 {
		t.Fatalf("loadFromEnv() size = %d, want 1", c.Size())
	}

	logs := buf.String()
	for _, want := range []string{
		"ignoring malformed tenant catalog env var",
		"tenant_Bad.Key",
		"ignoring tenant catalog env var with empty database name",
		"tenant_novalue",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs missing %q; got:\n%s", want, logs)
		}
	}
	if n := strings.Count(logs, "\"level\":\"WARN\""); n != 2 {
		t.Errorf("WARN count = %d, want 2 (unprefixed keys must not warn); got:\n%s", n, logs)
	}
}
