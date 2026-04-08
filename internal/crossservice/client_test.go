package crossservice

import (
	"log/slog"
	"testing"
	"time"
)

func TestResolveMode(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name      string
		mode      string
		override  string
		want      string
		wantError bool
	}{
		{"lite no override", "lite", "", "lite", false},
		{"lite override strict", "lite", "strict", "strict", false},
		{"strict override lite", "strict", "lite", "lite", false},
		{"none override strict rejected", "none", "strict", "", true},
		{"none override lite rejected", "none", "lite", "", true},
		{"none no override", "none", "", "none", false},
		{"invalid override", "lite", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.mode, "http://localhost:9100/mcp", 500*time.Millisecond, logger)
			got, err := c.ResolveMode(tt.override)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
