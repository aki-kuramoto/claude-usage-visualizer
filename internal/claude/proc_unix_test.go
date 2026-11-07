//go:build !windows

package claude

import (
	"testing"
	"time"
)

func TestParsePSTime(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"0:03.62":    3620 * time.Millisecond,
		"1:02.50":    62500 * time.Millisecond,
		"01:00:00":   time.Hour,
		"2-01:00:00": 49 * time.Hour,
		"":           -1,
		"garbage":    -1,
	} {
		if got := parsePSTime(in); got != want {
			t.Errorf("parsePSTime(%q) = %v, want %v", in, got, want)
		}
	}
}
