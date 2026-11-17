package tenancy

import (
	"context"
	"testing"
)

func TestIDFromContext(t *testing.T) {
	t.Run("missing tenant", func(t *testing.T) {
		id, ok := IDFromContext(context.Background())
		if ok || id != "" {
			t.Fatalf("IDFromContext() = %q, %v; want empty, false", id, ok)
		}
	})

	t.Run("with tenant", func(t *testing.T) {
		ctx := WithID(context.Background(), "acme")
		id, ok := IDFromContext(ctx)
		if !ok || id != "acme" {
			t.Fatalf("IDFromContext() = %q, %v; want acme, true", id, ok)
		}
	})

	t.Run("with raw header", func(t *testing.T) {
		ctx := WithRawHeader(context.Background(), "acme|user123")
		raw, ok := RawHeaderFromContext(ctx)
		if !ok || raw != "acme|user123" {
			t.Fatalf("RawHeaderFromContext() = %q, %v; want acme|user123, true", raw, ok)
		}
	})
}

func TestIDFromHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{name: "extracts tenant from composite header", header: "acme|user123", want: "acme", ok: true},
		{name: "trims whitespace", header: "  acme | user123 ", want: "acme", ok: true},
		{name: "tenant only", header: "acme", want: "acme", ok: true},
		{name: "empty header", header: "", ok: false},
		{name: "missing tenant segment", header: "|user123", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := IDFromHeader(tt.header)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("IDFromHeader(%q) = %q, %v; want %q, %v", tt.header, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("  Acme-Corp "); got != "acme-corp" {
		t.Fatalf("Normalize() = %q, want acme-corp", got)
	}
}
