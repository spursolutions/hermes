package sqlpool

import (
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestOptionsFrom_Defaults(t *testing.T) {
	got, err := optionsFrom(envOf(nil))
	if err != nil {
		t.Fatalf("optionsFrom() error = %v", err)
	}
	if got != DefaultOptions() {
		t.Fatalf("optionsFrom() = %+v, want defaults %+v", got, DefaultOptions())
	}
}

func TestOptionsFrom_Overrides(t *testing.T) {
	got, err := optionsFrom(envOf(map[string]string{
		EnvMaxConns:        "10",
		EnvMaxIdleConns:    "2",
		EnvConnMaxLifetime: "1m",
		EnvIdleTTL:         "10m",
		EnvSweepInterval:   "30s",
	}))
	if err != nil {
		t.Fatalf("optionsFrom() error = %v", err)
	}
	want := Options{MaxConns: 10, MaxIdleConns: 2, ConnMaxLifetime: time.Minute, IdleTTL: 10 * time.Minute, SweepInterval: 30 * time.Second}
	if got != want {
		t.Fatalf("optionsFrom() = %+v, want %+v", got, want)
	}
}

func TestOptionsFrom_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "non-numeric max conns", env: map[string]string{EnvMaxConns: "abc"}, wantErr: EnvMaxConns},
		{name: "zero max conns", env: map[string]string{EnvMaxConns: "0"}, wantErr: "must be at least 1"},
		{name: "negative idle conns", env: map[string]string{EnvMaxIdleConns: "-1"}, wantErr: "must be non-negative"},
		{name: "idle exceeds max", env: map[string]string{EnvMaxConns: "2", EnvMaxIdleConns: "3"}, wantErr: "cannot exceed"},
		{name: "bad duration", env: map[string]string{EnvIdleTTL: "soon"}, wantErr: EnvIdleTTL},
		{name: "negative lifetime", env: map[string]string{EnvConnMaxLifetime: "-1s"}, wantErr: EnvConnMaxLifetime},
		{name: "zero idle ttl", env: map[string]string{EnvIdleTTL: "0s"}, wantErr: "greater than zero"},
		{name: "zero sweep", env: map[string]string{EnvSweepInterval: "0s"}, wantErr: EnvSweepInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := optionsFrom(envOf(tt.env))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "%!") {
				t.Fatalf("error has a formatting artifact: %q", err)
			}
		})
	}
}
