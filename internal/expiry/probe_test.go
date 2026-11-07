package expiry

import (
	"context"
	"os"
	"testing"
	"time"
)

// The test binary itself plays claude: TestMain turns into the fake when
// FAKE_CLAUDE names what it should draw. Each fake stays running, as claude
// does, so Probe has to stop it.
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_CLAUDE") {
	case "":
		os.Exit(m.Run())
	case "warning":
		os.Stdout.WriteString("Your\x1b[1Clogin\x1b[1Cexpires\x1b[1Cin\x1b[1C2\x1b[1Cdays\r\n")
	case "ready":
		os.Stdout.WriteString("Claude Code\r\n\x1b[1C\u276f \r\n")
	case "tty":
		if isTerminal() {
			os.Stdout.WriteString("\u276f on a terminal\r\n")
		}
	case "silent":
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// fake returns a claude that draws what is named.
func fake(t *testing.T, what string) (path string, env []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe, append(os.Environ(), "FAKE_CLAUDE="+what)
}

func TestProbe(t *testing.T) {
	saved, savedTimeout := Settle, Timeout
	Settle, Timeout = 300*time.Millisecond, 10*time.Second
	defer func() { Settle, Timeout = saved, savedTimeout }()

	tests := []struct {
		name, what string
		want       Status
		days       int
	}{
		{"warning", "warning", StatusDays, 2},
		{"ready", "ready", StatusNone, 0},
		{"tty", "tty", StatusNone, 0},
		{"silent", "silent", StatusUnknown, 0},
	}
	for _, tt := range tests {
		start := time.Now()
		path, env := fake(t, tt.what)
		r, err := Probe(context.Background(), path, t.TempDir(), env)
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
	path, env := fake(t, "silent")
	_, err := Probe(ctx, path, t.TempDir(), env)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Errorf("err %v after %v", err, time.Since(start))
	}
}
