//go:build !windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/days"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/expiry"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

// fakeClaude answers /usage with fixed text, or exits when the file "fail"
// exists in its working directory.
const fakeClaude = `#!/bin/sh
echo '{"type":"system","subtype":"init","claude_code_version":"9.9.9"}'
while IFS= read -r line; do
  if [ -e fail ]; then echo "broken on purpose" >&2; exit 3; fi
  echo '{"type":"assistant","usage_report":{"rate_limits":{}}}'
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"local_command":"usage","result":"You are currently using your subscription to power your Claude Code usage\n\nCurrent session: 35% used · resets Oct 15 at 2:20pm (Asia/Tokyo)\n"}'
done
`

type recorder struct {
	mu     sync.Mutex
	states []State
}

func (r *recorder) publish(s State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, s)
}

func (r *recorder) waitFor(t *testing.T, cond func(State) bool) State {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.states {
			if cond(s) {
				r.mu.Unlock()
				return s
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("state never reached")
	return State{}
}

func start(t *testing.T) (*Controller, *recorder, paths.Paths, context.CancelFunc) {
	t.Helper()
	// Generous: macOS checks a newly written executable the first time it
	// runs, which can take seconds on a busy machine.
	claudeTimeout = 15 * time.Second
	p := paths.Paths{Base: t.TempDir()}
	fake := filepath.Join(p.Base, "claude")
	if err := os.WriteFile(fake, []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	s := settings.Default()
	s.ClaudePath = fake
	c := New(p, oplog.New(p.Logs(), nil, nil), s, rec.publish)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return c, rec, p, cancel
}

func readLogs(t *testing.T, dir, prefix string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, prefix+"-*.jsonl"))
	var b strings.Builder
	for _, f := range files {
		data, _ := os.ReadFile(f)
		b.Write(data)
	}
	return b.String()
}

func TestStartsPendingThenReads(t *testing.T) {
	_, rec, p, _ := start(t)
	defer func() {
		if t.Failed() {
			t.Logf("ops:\n%s", readLogs(t, p.Logs(), "ops"))
		}
	}()
	first := rec.waitFor(t, func(State) bool { return true })
	if first.Status != StatusPending || len(first.Limits) != 0 {
		t.Errorf("first state = %+v", first)
	}
	ok := rec.waitFor(t, func(s State) bool { return s.Status == StatusOK })
	if len(ok.Limits) != 1 || ok.Limits[0].UsedPct != 35 || ok.Limits[0].ResetsAt == "" || ok.LastSuccessAt == "" {
		t.Errorf("ok state = %+v", ok)
	}
	usageLines := readLogs(t, p.Logs(), "usage")
	if strings.Count(usageLines, "\n") != 1 || !strings.Contains(usageLines, `"ok":true`) {
		t.Errorf("usage record:\n%s", usageLines)
	}
	ops := readLogs(t, p.Logs(), "ops")
	for _, kind := range []string{"app_start", "claude_found", "claude_start", "fetch_start", "fetch_end", "claude_init"} {
		if !strings.Contains(ops, `"kind":"`+kind+`"`) {
			t.Errorf("no %s in ops:\n%s", kind, ops)
		}
	}
	if !strings.Contains(ops, `"usage_report":{"rate_limits":{}}`) || !strings.Contains(ops, `"reason":"start"`) {
		t.Errorf("fetch details missing:\n%s", ops)
	}
}

func TestFailureKeepsLastValues(t *testing.T) {
	c, rec, p, _ := start(t)
	ok := rec.waitFor(t, func(s State) bool { return s.Status == StatusOK })
	os.MkdirAll(p.Work(), 0o755)
	os.WriteFile(filepath.Join(p.Work(), "fail"), nil, 0o644)
	c.FetchNow("manual")
	failed := rec.waitFor(t, func(s State) bool { return s.Status == StatusFailed && !s.Fetching })
	if len(failed.Limits) != 1 || failed.Limits[0].UsedPct != 35 || !strings.Contains(failed.Error, "exited") {
		t.Errorf("failed state = %+v", failed)
	}
	// Times have one-second resolution, so compare with the successful
	// reading rather than expecting the two times to differ.
	if failed.LastSuccessAt != ok.LastSuccessAt || failed.LastAttemptAt == "" {
		t.Errorf("last success should stay at the earlier reading %s: %+v", ok.LastSuccessAt, failed)
	}
	usageLines := readLogs(t, p.Logs(), "usage")
	if !strings.Contains(usageLines, `"stage":"wait","reason":"exited"`) {
		t.Errorf("usage record:\n%s", usageLines)
	}
}

func TestSaveSettings(t *testing.T) {
	c, rec, p, _ := start(t)
	rec.waitFor(t, func(s State) bool { return s.Status == StatusOK })
	s := c.State().Settings
	s.IntervalMinutes = 7
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	rec.waitFor(t, func(s State) bool { return s.Settings.IntervalMinutes == 7 })
	saved, err := settings.Load(p.Settings())
	if err != nil || saved.IntervalMinutes != 7 {
		t.Errorf("saved %+v, %v", saved, err)
	}
	s.IntervalMinutes = 0
	if err := c.SaveSettings(s); err == nil {
		t.Error("an interval of 0 was saved")
	}
}

func TestDaysElapsed(t *testing.T) {
	c, _, p, _ := start(t)
	reset := time.Date(2026, 10, 22, 7, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.Local)
	week := 7 * 24 * time.Hour

	// With no days.setay, Run writes the default one, which is then used.
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(readLogs(t, p.Logs(), "ops"), `"kind":"days_created"`) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if data, err := os.ReadFile(p.Days()); err != nil || string(data) != days.DefaultText {
		t.Fatalf("the default was not written: %v", err)
	}
	if _, ok := c.DaysElapsed(reset, week, now); !ok {
		t.Error("no value from the default days.setay")
	}
	time.Sleep(10 * time.Millisecond) // a new modification time for the next write

	all := "{ kinds = { a = 1; }; weekdays = { mon='a'; tue='a'; wed='a'; thu='a'; fri='a'; sat='a'; sun='a'; }; }"
	os.WriteFile(p.Days(), []byte(all), 0o644)
	got, ok := c.DaysElapsed(reset, week, now)
	plain := 1 - reset.Sub(now).Seconds()/week.Seconds()
	if !ok || got < plain-1e-9 || got > plain+1e-9 {
		t.Errorf("uniform days: %v %v, want %v", got, ok, plain)
	}
	if _, ok := c.DaysElapsed(reset, 5*time.Hour, now); ok {
		t.Error("a value for the five-hour limit")
	}

	// Editing the file is picked up; a broken file gives an error and no value.
	time.Sleep(10 * time.Millisecond) // a new modification time
	os.WriteFile(p.Days(), []byte("{ kinds = { a = 1; }; }"), 0o644)
	if _, ok := c.DaysElapsed(reset, week, now); ok || c.DaysError() == "" {
		t.Errorf("broken file: ok=%v error=%q", ok, c.DaysError())
	}
	if ops := readLogs(t, p.Logs(), "ops"); !strings.Contains(ops, `"kind":"days_error"`) || !strings.Contains(ops, `"kind":"days_loaded"`) {
		t.Errorf("ops:\n%s", ops)
	}
}

func TestMovePastDays(t *testing.T) {
	c, _, p, _ := start(t)
	src := "{\n\tkinds = { a = 1; o = 0; };\n\tweekdays = { mon='a'; tue='a'; wed='a'; thu='a'; fri='a'; sat='o'; sun='o'; };\n" +
		"\tdays = {\n\t\to = [\n\t\t\t'2000-01-01', # long ago\n\t\t\t'2999-01-01',\n\t\t];\n\t};\n}\n"
	os.WriteFile(p.Days(), []byte(src), 0o644)
	c.movePastDays(time.Now())
	got, _ := os.ReadFile(p.Days())
	if !strings.Contains(string(got), "past_days = {\n\t\to = [\n\t\t\t'2000-01-01', # long ago\n") || strings.Count(string(got), "2999-01-01") != 1 {
		t.Errorf("got\n%s", got)
	}
	if ops := readLogs(t, p.Logs(), "ops"); !strings.Contains(ops, `"kind":"days_moved"`) {
		t.Errorf("ops:\n%s", ops)
	}
}

// loginClaude answers like claude logged in when the file "loggedin" exists
// in its working directory, and like claude logged out otherwise.
const loginClaude = `#!/bin/sh
if [ "$1" = auth ]; then
  if [ -e loggedin ]; then echo '{"loggedIn":true,"authMethod":"claude.ai","subscriptionType":"team","email":"x@example.com"}'; exit 0; fi
  echo '{"loggedIn":false,"authMethod":"none"}'; exit 1
fi
echo '{"type":"system","subtype":"init","claude_code_version":"9.9.9"}'
while IFS= read -r line; do
  if [ -e loggedin ]; then
    printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"local_command":"usage","result":"You are currently using your subscription to power your Claude Code usage\n\nCurrent session: 5% used · resets Oct 15 at 2:20pm (Asia/Tokyo)\n"}'
  else
    printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"local_command":"usage","result":"Total cost:            $0.0000\nTotal duration (API):  0s\n"}'
  fi
done
`

func TestLoggedOutAndBack(t *testing.T) {
	saved := minuteTick
	minuteTick = 200 * time.Millisecond
	defer func() { minuteTick = saved }()

	claudeTimeout = 15 * time.Second
	p := paths.Paths{Base: t.TempDir()}
	fake := filepath.Join(p.Base, "claude")
	os.WriteFile(fake, []byte(loginClaude), 0o755)
	os.MkdirAll(p.Work(), 0o755)
	rec := &recorder{}
	s := settings.Default()
	s.ClaudePath = fake
	c := New(p, oplog.New(p.Logs(), nil, nil), s, rec.publish)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	out := rec.waitFor(t, func(s State) bool { return s.Status == StatusFailed })
	if out.ErrorReason != "not_logged_in" || out.ErrorStage != "auth" {
		t.Errorf("logged out state: %+v", out)
	}
	if u := readLogs(t, p.Logs(), "usage"); !strings.Contains(u, `"reason":"not_logged_in"`) {
		t.Errorf("usage record:\n%s", u)
	}

	// Logging in elsewhere is noticed without a scheduled reading.
	os.WriteFile(filepath.Join(p.Work(), "loggedin"), nil, 0o644)
	back := rec.waitFor(t, func(s State) bool { return s.Status == StatusOK })
	if len(back.Limits) != 1 || back.ErrorReason != "" {
		t.Errorf("back: %+v", back)
	}
	ops := readLogs(t, p.Logs(), "ops")
	if !strings.Contains(ops, `"reason":"login_restored"`) || strings.Contains(ops, "x@example.com") {
		t.Errorf("ops:\n%s", ops)
	}
}

// expiryClaude is logged in, and on the interactive screen (started with
// --strict-mcp-config alone) warns that the login expires in 2 days.
const expiryClaude = `#!/bin/sh
if [ "$1" = auth ]; then echo '{"loggedIn":true,"authMethod":"claude.ai"}'; exit 0; fi
if [ "$1" = --strict-mcp-config ] && [ $# = 1 ]; then
  printf 'Your\033[1Clogin\033[1Cexpires\033[1Cin\033[1C2\033[1Cdays\r\n❯ \r\n'; sleep 60
fi
sleep 60
`

func TestCheckExpiry(t *testing.T) {
	savedSettle := expiry.Settle
	expiry.Settle = 300 * time.Millisecond
	defer func() { expiry.Settle = savedSettle }()

	p := paths.Paths{Base: t.TempDir()}
	fake := filepath.Join(p.Base, "claude")
	os.WriteFile(fake, []byte(expiryClaude), 0o755)
	s := settings.Default()
	s.ClaudePath = fake
	rec := &recorder{}
	c := New(p, oplog.New(p.Logs(), nil, nil), s, rec.publish)

	if got := c.checkExpiry(context.Background(), "test"); got != expiry.StatusDays {
		t.Fatalf("got %q", got)
	}
	st := c.State()
	if st.LoginExpiry == nil || st.LoginExpiry.Status != expiry.StatusDays || st.LoginExpiry.DaysLeft != 2 {
		t.Errorf("state: %+v", st.LoginExpiry)
	}
	if ops := readLogs(t, p.Logs(), "ops"); !strings.Contains(ops, `"kind":"login_expiry"`) || !strings.Contains(ops, `"days_left":2`) {
		t.Errorf("ops:\n%s", ops)
	}

	// Turned off: no look, and the result is cleared.
	s.LoginExpiryCheck = false
	c.applySettings(s)
	if got := c.checkExpiry(context.Background(), "test"); got != "" || c.State().LoginExpiry != nil {
		t.Errorf("off: %q %+v", got, c.State().LoginExpiry)
	}
}
