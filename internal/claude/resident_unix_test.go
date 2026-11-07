//go:build !windows

package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/usage"
)

// fakeClaude answers each input line the way claude answers /usage, or, by
// FAKE_MODE, hangs or exits.
const fakeClaude = `#!/bin/sh
echo '{"type":"system","subtype":"init","claude_code_version":"9.9.9","model":"haiku","cwd":"/x"}'
n=0
while IFS= read -r line; do
  n=$((n+1))
  case "$FAKE_MODE" in
    hang) sleep 30 ;;
    exit) echo "something broke" >&2; exit 3 ;;
    *)
      echo '{"type":"system","subtype":"hook_started"}'
      echo '{"type":"assistant","usage_report":{"rate_limits":{"n":'$n'}}}'
      printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"local_command":"usage","result":"Current session: '$n'% used"}'
      ;;
  esac
done
`

func setup(t *testing.T, mode string) (*Resident, string) {
	timeout := 10 * time.Second
	if mode == "hang" {
		timeout = 500 * time.Millisecond
	}
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(dir, "logs")
	r := New(Config{
		Path:    path,
		Dir:     dir,
		Env:     append(Env(os.Environ(), path), "FAKE_MODE="+mode),
		Timeout: timeout,
		Log:     oplog.New(logs, nil, nil),
	})
	t.Cleanup(r.Stop)
	return r, logs
}

func opsText(t *testing.T, logs string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(logs, "ops-*.jsonl"))
	var b strings.Builder
	for _, f := range files {
		data, _ := os.ReadFile(f)
		b.Write(data)
	}
	return b.String()
}

func TestAnswersRepeatedly(t *testing.T) {
	r, logs := setup(t, "ok")
	for i, want := range []string{"Current session: 1% used", "Current session: 2% used"} {
		ans, err := r.Ask("id")
		if err != nil {
			t.Fatalf("request %d: %+v", i, err)
		}
		if ans.Text != want || ans.LocalCommand != "usage" || !strings.Contains(string(ans.UsageReport), `"n":`) {
			t.Errorf("request %d: %+v", i, ans)
		}
	}
	ops := opsText(t, logs)
	if strings.Count(ops, `"kind":"claude_start"`) != 1 || !strings.Contains(ops, `"version":"9.9.9"`) {
		t.Errorf("ops:\n%s", ops)
	}
}

func TestTimeoutKillsAndRestarts(t *testing.T) {
	r, logs := setup(t, "hang")
	_, err := r.Ask("first")
	if err == nil || err.Stage != usage.StageWait || err.Reason != usage.ReasonTimeout {
		t.Fatalf("got %+v", err)
	}
	if _, err := r.Ask("second"); err == nil || err.Reason != usage.ReasonTimeout {
		t.Fatalf("got %+v", err)
	}
	ops := opsText(t, logs)
	for _, kind := range []string{"claude_kill", "claude_exit"} {
		if strings.Count(ops, `"kind":"`+kind+`"`) < 2 {
			t.Errorf("expected two %s events:\n%s", kind, ops)
		}
	}
	if strings.Count(ops, `"kind":"claude_start"`) != 2 {
		t.Errorf("expected a restart:\n%s", ops)
	}
}

func TestExitWhileAnswering(t *testing.T) {
	r, logs := setup(t, "exit")
	_, err := r.Ask("id")
	if err == nil || err.Reason != usage.ReasonExited {
		t.Fatalf("got %+v", err)
	}
	// The exit event is written by another goroutine; give it a moment.
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(opsText(t, logs), "something broke") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if ops := opsText(t, logs); !strings.Contains(ops, `"code":3`) || !strings.Contains(ops, "something broke") {
		t.Errorf("exit not logged with its code and stderr:\n%s", ops)
	}
}

func TestStartFailure(t *testing.T) {
	r := New(Config{Path: filepath.Join(t.TempDir(), "missing"), Log: oplog.New(t.TempDir(), nil, nil)})
	if _, err := r.Ask("id"); err == nil || err.Stage != usage.StageSpawn || err.Reason != usage.ReasonStartFailed {
		t.Errorf("got %+v", err)
	}
}

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
