package claude

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/usage"
)

// The test binary itself plays claude: TestMain turns into fakeClaude when
// FAKE_CLAUDE is set. It answers each input line the way claude answers
// /usage, or, by FAKE_MODE, hangs or exits.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_CLAUDE") == "1" {
		fakeClaude(os.Getenv("FAKE_MODE"))
		return
	}
	os.Exit(m.Run())
}

func fakeClaude(mode string) {
	fmt.Println(`{"type":"system","subtype":"init","claude_code_version":"9.9.9","model":"haiku","cwd":"/x"}`)
	in := bufio.NewScanner(os.Stdin)
	for n := 1; in.Scan(); n++ {
		switch mode {
		case "hang":
			time.Sleep(30 * time.Second)
		case "exit":
			fmt.Fprintln(os.Stderr, "something broke")
			os.Exit(3)
		default:
			// Use some processor time, as /usage does, so that it can be
			// measured.
			for end := time.Now().Add(50 * time.Millisecond); time.Now().Before(end); {
			}
			fmt.Println(`{"type":"system","subtype":"hook_started"}`)
			fmt.Printf(`{"type":"assistant","usage_report":{"rate_limits":{"n":%d}}}`+"\n", n)
			fmt.Printf(`{"type":"result","subtype":"success","is_error":false,"local_command":"usage","result":"Current session: %d%% used"}`+"\n", n)
		}
	}
}

// fakePath returns the program to start as claude: the test binary, or on
// Windows optionally a .cmd script that starts it, as npm installs claude.
func fakePath(t *testing.T, dir string, viaCmd bool) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !viaCmd {
		return exe
	}
	path := filepath.Join(dir, "claude.cmd")
	script := "@echo off\r\n\"" + exe + "\" %*\r\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func setup(t *testing.T, mode string) (*Resident, string) {
	return setupWith(t, mode, false)
}

func setupWith(t *testing.T, mode string, viaCmd bool) (*Resident, string) {
	timeout := 10 * time.Second
	if mode == "hang" {
		timeout = 500 * time.Millisecond
	}
	t.Helper()
	dir := t.TempDir()
	path := fakePath(t, dir, viaCmd)
	logs := filepath.Join(dir, "logs")
	r := New(Config{
		Path:    path,
		Dir:     dir,
		Env:     append(Env(os.Environ(), path), "FAKE_CLAUDE=1", "FAKE_MODE="+mode),
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
	testAnswersRepeatedly(t, r, logs)
}

func testAnswersRepeatedly(t *testing.T, r *Resident, logs string) {
	for i, want := range []string{"Current session: 1% used", "Current session: 2% used"} {
		ans, err := r.Ask("id")
		if err != nil {
			t.Fatalf("request %d: %+v", i, err)
		}
		if ans.Text != want || ans.LocalCommand != "usage" || !strings.Contains(string(ans.UsageReport), `"n":`) {
			t.Errorf("request %d: %+v", i, ans)
		}
		// ps on Linux reports whole seconds, so only Windows (100 ns) is
		// sure to see the 50 ms the fake spends.
		if ans.CPU < 0 || (runtime.GOOS == "windows" && ans.CPU == 0) {
			t.Errorf("request %d: CPU time not measured: %v", i, ans.CPU)
		}
	}
	ops := opsText(t, logs)
	if strings.Count(ops, `"kind":"claude_start"`) != 1 || !strings.Contains(ops, `"version":"9.9.9"`) {
		t.Errorf("ops:\n%s", ops)
	}
}

func TestTimeoutKillsAndRestarts(t *testing.T) {
	r, logs := setup(t, "hang")
	testTimeoutKillsAndRestarts(t, r, logs)
}

func testTimeoutKillsAndRestarts(t *testing.T, r *Resident, logs string) {
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
