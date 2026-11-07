//go:build !windows

package expiry

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Settle is how long the screen has to stay unchanged before it is read as
// final, and Timeout how long a reading may take in all.
var (
	Settle  = 3 * time.Second
	Timeout = 30 * time.Second
)

// Probe starts the interactive claude at path in dir on a pseudo terminal,
// reads its screen until it settles or something definite shows, and stops
// it. Nothing is typed into claude: it is ended by a signal, before any
// conversation starts, so no session is saved.
//
// --strict-mcp-config keeps it from starting MCP servers, which reading the
// first screen does not need. The SessionStart hooks do run, as they do
// whenever claude starts.
//
// Ending ctx stops claude and returns at once.
func Probe(ctx context.Context, path, dir string, env []string) (Result, error) {
	cmd := exec.Command(path, "--strict-mcp-config")
	cmd.Dir = dir
	cmd.Env = append(env, "TERM=xterm-256color")
	// A session of its own, with the pseudo terminal as its controlling
	// terminal, as a program started in a terminal window has.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 40})
	if err != nil {
		return Result{}, err
	}
	defer func() {
		// The whole session, so that hooks started by claude go too.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
		f.Close()
	}()

	var (
		mu      sync.Mutex
		raw     []byte
		changed = time.Now()
		closed  = make(chan struct{})
	)
	go func() {
		buf := make([]byte, 16<<10)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				mu.Lock()
				raw = append(raw, buf[:n]...)
				changed = time.Now()
				mu.Unlock()
			}
			if err != nil {
				close(closed)
				return
			}
		}
	}()

	deadline := time.Now().Add(Timeout)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return Result{Status: StatusUnknown}, ctx.Err()
		case <-closed:
			mu.Lock()
			r := Classify(raw, true)
			mu.Unlock()
			return r, nil
		case now := <-tick.C:
			mu.Lock()
			settled := now.Sub(changed) >= Settle && len(raw) > 0
			r := Classify(raw, settled)
			mu.Unlock()
			if r.Status != StatusUnknown || settled {
				return r, nil
			}
			if now.After(deadline) {
				mu.Lock()
				r = Classify(raw, true)
				mu.Unlock()
				return r, nil
			}
		}
	}
}
