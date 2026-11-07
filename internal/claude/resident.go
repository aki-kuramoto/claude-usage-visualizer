// Package claude keeps one claude process running and asks it for /usage.
//
// claude is started with stream-json on both sides, so that it stays alive
// between requests: each request is one JSON line on its standard input, and
// the answer arrives as JSON lines on its standard output. /usage is a local
// command, so no request reaches the model.
package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/usage"
)

// Args are the arguments claude is started with.
//
//   - -p with stream-json input and output keeps one process answering many
//     requests.
//   - --verbose is required by stream-json output.
//   - --no-session-persistence leaves no transcript behind.
//   - --strict-mcp-config without --mcp-config starts no MCP servers; /usage
//     needs none, and they would cost memory and start-up time.
//   - --model haiku keeps the cost small should /usage ever reach the model
//     instead of being handled locally.
var Args = []string{
	"-p",
	"--input-format", "stream-json",
	"--output-format", "stream-json",
	"--verbose",
	"--no-session-persistence",
	"--strict-mcp-config",
	"--model", "haiku",
}

// DefaultTimeout is how long to wait for the answer to one request.
const DefaultTimeout = 60 * time.Second

// Config says how to run claude.
type Config struct {
	Path    string
	Dir     string
	Env     []string
	Timeout time.Duration // DefaultTimeout when zero
	Log     *oplog.Log
}

// Answer is what claude returned for one /usage request.
type Answer struct {
	// Text is the "result" of the result event: the /usage text.
	Text string
	// UsageReport is the structured report attached to the assistant event,
	// kept as it came.
	UsageReport json.RawMessage
	// LocalCommand is the command claude says it ran; "usage" when /usage
	// was handled locally.
	LocalCommand string
	// CPU is the processor time claude spent on the request, or -1 when it
	// could not be measured.
	CPU time.Duration
}

// Resident runs claude and sends it requests one at a time.
type Resident struct {
	cfg  Config
	mu   sync.Mutex
	proc *process
}

// New returns a Resident; claude is started by the first request.
func New(cfg Config) *Resident {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Resident{cfg: cfg}
}

// Ask sends /usage and waits for the answer. id ties the log events to the
// reading. A timed-out claude is stopped, and the next request starts a new
// one.
func (r *Resident) Ask(id string) (*Answer, *usage.Error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.proc == nil || r.proc.hasExited() {
		p, err := r.start(id)
		if err != nil {
			return nil, err
		}
		r.proc = p
	}
	p := r.proc
	p.drain()

	cpuBefore := cpuTime(p)
	line, _ := json.Marshal(map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": "/usage"},
	})
	if _, err := p.stdin.Write(append(line, '\n')); err != nil {
		return nil, &usage.Error{Stage: usage.StageSend, Reason: usage.ReasonExited,
			Detail: fmt.Sprintf("writing the request: %v", err)}
	}

	ans := &Answer{CPU: -1}
	timer := time.NewTimer(r.cfg.Timeout)
	defer timer.Stop()
	for {
		select {
		case ev := <-p.events:
			switch ev.Type {
			case "assistant":
				if len(ev.UsageReport) > 0 {
					ans.UsageReport = ev.UsageReport
				}
			case "result":
				ans.Text = ev.Result
				ans.LocalCommand = ev.LocalCommand
				if after := cpuTime(p); cpuBefore >= 0 && after >= 0 {
					ans.CPU = after - cpuBefore
				}
				if ev.IsError {
					return ans, &usage.Error{Stage: usage.StageWait, Reason: usage.ReasonResultError,
						Detail: fmt.Sprintf("claude reported an error (subtype %q)", ev.Subtype)}
				}
				return ans, nil
			}
		case <-p.exited:
			return nil, &usage.Error{Stage: usage.StageWait, Reason: usage.ReasonExited,
				Detail: fmt.Sprintf("claude exited while answering: %v", p.exitErr)}
		case <-timer.C:
			r.kill(p, id, "timeout")
			return nil, &usage.Error{Stage: usage.StageWait, Reason: usage.ReasonTimeout,
				Detail: fmt.Sprintf("no result within %s", r.cfg.Timeout)}
		}
	}
}

// Stop ends claude, first by closing its input and then, if it does not
// leave, by force.
func (r *Resident) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proc == nil || r.proc.hasExited() {
		return
	}
	r.proc.stdin.Close()
	select {
	case <-r.proc.exited:
	case <-time.After(5 * time.Second):
		r.kill(r.proc, "", "did not exit after its input was closed")
	}
}

func (r *Resident) start(id string) (*process, *usage.Error) {
	cmd := exec.Command(r.cfg.Path, Args...)
	cmd.Dir = r.cfg.Dir
	cmd.Env = r.cfg.Env
	prepare(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, startError(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, startError(err)
	}
	p := &process{
		cmd:    cmd,
		stdin:  stdin,
		events: make(chan event, 64),
		exited: make(chan struct{}),
		stderr: &tail{max: 16 << 10},
	}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, startError(err)
	}
	started(p)
	r.cfg.Log.Add(oplog.KindClaudeStart, id, map[string]any{
		"path": r.cfg.Path, "args": Args, "dir": r.cfg.Dir, "pid": cmd.Process.Pid,
	})
	go p.read(stdout, r.cfg.Log)
	go func() {
		p.exitErr = cmd.Wait()
		exited(p)
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		r.cfg.Log.Add(oplog.KindClaudeExit, "", map[string]any{
			"pid": cmd.Process.Pid, "code": code, "error": errString(p.exitErr), "stderr": p.stderr.String(),
		})
		close(p.exited)
	}()
	return p, nil
}

func (r *Resident) kill(p *process, id, reason string) {
	r.cfg.Log.Add(oplog.KindClaudeKill, id, map[string]any{"pid": p.pid(), "reason": reason})
	killTree(p)
	select {
	case <-p.exited:
	case <-time.After(5 * time.Second):
		r.cfg.Log.Add(oplog.KindError, id, map[string]any{
			"what": "claude did not exit after being killed", "pid": p.pid()})
	}
}

func startError(err error) *usage.Error {
	return &usage.Error{Stage: usage.StageSpawn, Reason: usage.ReasonStartFailed, Detail: err.Error()}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// event is the part of a stream-json line this package looks at.
type event struct {
	Type         string          `json:"type"`
	Subtype      string          `json:"subtype"`
	Result       string          `json:"result"`
	IsError      bool            `json:"is_error"`
	LocalCommand string          `json:"local_command"`
	UsageReport  json.RawMessage `json:"usage_report"`
	Version      string          `json:"claude_code_version"`
	Model        string          `json:"model"`
	Cwd          string          `json:"cwd"`
}

type process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	events  chan event
	exited  chan struct{}
	exitErr error
	stderr  *tail
	sys     procSys
}

func (p *process) pid() int { return p.cmd.Process.Pid }

func (p *process) hasExited() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

// drain drops events left over from an earlier request.
func (p *process) drain() {
	for {
		select {
		case <-p.events:
		default:
			return
		}
	}
}

// read turns standard output into events. Lines can be long (the init event
// lists every tool), so it reads whole lines rather than using a Scanner with
// a fixed limit.
func (p *process) read(stdout io.Reader, log *oplog.Log) {
	br := bufio.NewReaderSize(stdout, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var ev event
			if jerr := json.Unmarshal(line, &ev); jerr != nil {
				log.Add(oplog.KindError, "", map[string]any{
					"what": "claude printed a line that is not JSON", "line": truncate(string(line), 2000)})
			} else if ev.Type == "system" && ev.Subtype == "init" {
				log.Add(oplog.KindClaudeInit, "", map[string]any{
					"pid": p.pid(), "version": ev.Version, "model": ev.Model, "cwd": ev.Cwd})
			} else if ev.Type == "assistant" || ev.Type == "result" {
				select {
				case p.events <- ev:
				default:
					log.Add(oplog.KindError, "", map[string]any{"what": "dropped an event nobody was waiting for", "type": ev.Type})
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Add(oplog.KindError, "", map[string]any{"what": "reading claude's output", "error": err.Error()})
			}
			return
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// tail keeps the last max bytes written to it.
type tail struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
