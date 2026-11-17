package sqlpool

import (
	"fmt"
	"strings"
)

// BaseConnString is the shared SQL Server connectivity baseline (server,
// port, credentials, encryption flags), typically sourced from a secret
// store. The tenant database name is composed onto this baseline at
// request time so that one shared secret serves all tenants.
type BaseConnString string

// ForDatabase returns a complete SQL connection string with the database
// attribute set to dbName. Any database or initial catalog attribute
// already present in the base connection string is replaced rather than
// duplicated.
func (b BaseConnString) ForDatabase(dbName string) string {
	parts := strings.Split(string(b), ";")
	out := make([]string, 0, len(parts)+1)
	replaced := false

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			out = append(out, trimmed)
			continue
		}
		if isDatabaseKey(strings.ToLower(strings.TrimSpace(key))) {
			out = append(out, fmt.Sprintf("database=%s", dbName))
			replaced = true
			continue
		}
		out = append(out, trimmed)
	}

	if !replaced {
		out = append(out, fmt.Sprintf("database=%s", dbName))
	}

	return strings.Join(out, ";")
}

func isDatabaseKey(key string) bool {
	switch key {
	case "database", "initial catalog":
		return true
	}
	return false
}
