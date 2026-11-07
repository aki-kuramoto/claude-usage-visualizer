//go:build !windows

package expiry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fake(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProbe(t *testing.T) {
	saved, savedTimeout := Settle, Timeout
	Settle, Timeout = 300*time.Millisecond, 10*time.Second
	defer func() { Settle, Timeout = saved, savedTimeout }()

	tests := []struct {
		name, body string
		want       Status
		days       int
	}{
		// Each fake stays running, as claude does, so Probe has to stop it.
		{"warning", `printf 'Your\033[1Clogin\033[1Cexpires\033[1Cin\033[1C2\033[1Cdays\r\n'; sleep 60`, StatusDays, 2},
		{"ready", `printf 'Claude Code\r\n\033[1C❯ \r\n'; sleep 60`, StatusNone, 0},
		{"tty", `[ -t 0 ] && [ -t 1 ] && printf '❯ on a terminal\r\n'; sleep 60`, StatusNone, 0},
		{"silent", `sleep 60`, StatusUnknown, 0},
	}
	for _, tt := range tests {
		start := time.Now()
		r, err := Probe(context.Background(), fake(t, tt.body), t.TempDir(), os.Environ())
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if r.Status != tt.want || r.DaysLeft != tt.days {
			t.Errorf("%s: got %+v, want %s %d", tt.name, r, tt.want, tt.days)
		}
		if d := time.Since(start); d > 12*time.Second {
			t.Errorf("%s: took %v", tt.name, d)
		}
	}
}

func TestProbeStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := Probe(ctx, fake(t, "sleep 60"), t.TempDir(), os.Environ())
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err %v after %v", err, time.Since(start))
	}
}
