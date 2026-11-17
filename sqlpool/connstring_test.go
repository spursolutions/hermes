package sqlpool

import (
	"strings"
	"testing"
)

func TestBaseConnString_ForDatabase(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		dbName string
		want   string
	}{
		{
			name:   "replaces existing database attribute",
			base:   "server=sql.test;port=1433;database=Placeholder;user id=sa;password=secret",
			dbName: "AcmeDB",
			want:   "server=sql.test;port=1433;database=AcmeDB;user id=sa;password=secret",
		},
		{
			name:   "replaces initial catalog attribute",
			base:   "Server=sql.test;Initial Catalog=Placeholder;User Id=sa",
			dbName: "AcmeDB",
			want:   "Server=sql.test;database=AcmeDB;User Id=sa",
		},
		{
			name:   "appends database when absent",
			base:   "server=sql.test;user id=sa;password=secret",
			dbName: "AcmeDB",
			want:   "server=sql.test;user id=sa;password=secret;database=AcmeDB",
		},
		{
			name:   "preserves attribute order otherwise",
			base:   "server=sql.test;port=1433;encrypt=true;database=Placeholder;user id=sa",
			dbName: "GlobexDB",
			want:   "server=sql.test;port=1433;encrypt=true;database=GlobexDB;user id=sa",
		},
		{
			name:   "trims empty segments and whitespace",
			base:   "server=sql.test ; ; database=Placeholder ; user id=sa",
			dbName: "AcmeDB",
			want:   "server=sql.test;database=AcmeDB;user id=sa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BaseConnString(tt.base).ForDatabase(tt.dbName)
			if got != tt.want {
				t.Fatalf("ForDatabase() = %q, want %q", got, tt.want)
			}

			lower := strings.ToLower(got)
			if strings.Count(lower, "database=") != 1 {
				t.Fatalf("expected exactly one database= attribute in %q", got)
			}
			if strings.Count(lower, "initial catalog=") != 0 {
				t.Fatalf("expected initial catalog to be removed in %q", got)
			}
		})
	}
}
