//go:build !windows

package expiry

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type unixTerminal struct {
	*os.File
	cmd *exec.Cmd
}

func start(path, dir string, env []string) (terminal, error) {
	cmd := exec.Command(path, "--strict-mcp-config")
	cmd.Dir = dir
	cmd.Env = append(env, "TERM=xterm-256color")
	// A session of its own, with the pseudo terminal as its controlling
	// terminal, as a program started in a terminal window has.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 40})
	if err != nil {
		return nil, err
	}
	return &unixTerminal{File: f, cmd: cmd}, nil
}

// stop ends claude by a signal to its whole session, so that hooks started
// by claude go too.
func (t *unixTerminal) stop() {
	syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
	t.cmd.Wait()
	t.File.Close()
}
