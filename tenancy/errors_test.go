package tenancy

import (
	"errors"
	"fmt"
	"testing"
)

func TestUnknown_MatchesSentinel(t *testing.T) {
	err := Unknown("rogue")
	if !errors.Is(err, ErrUnknown) {
		t.Fatal("Unknown() must match ErrUnknown")
	}
	if err.Error() != "unknown tenant: rogue" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestClientMessage(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   string
		wantOK bool
	}{
		{name: "missing", err: ErrMissing, want: "tenant identity is required", wantOK: true},
		{name: "unknown", err: Unknown("rogue"), want: "unknown tenant: rogue", wantOK: true},
		{name: "wrapped missing", err: fmt.Errorf("repo: %w", ErrMissing), want: "tenant identity is required", wantOK: true},
		{name: "wrapped unknown", err: fmt.Errorf("repo: %w", Unknown("rogue")), want: "unknown tenant: rogue", wantOK: true},
		{name: "unrelated", err: errors.New("boom"), wantOK: false},
		{name: "nil", err: nil, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClientMessage(tt.err)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("ClientMessage() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
