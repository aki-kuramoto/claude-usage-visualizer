package expiry

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// Settle is how long the screen has to stay unchanged before it is read as
// final, and Timeout how long a reading may take in all.
var (
	Settle  = 3 * time.Second
	Timeout = 30 * time.Second
)

// terminal is claude running on a pseudo terminal: what it draws can be
// read, and stop ends it with everything it started.
type terminal interface {
	io.Reader
	stop()
}

// errUnsupported is returned by start when this system has no pseudo
// terminal to start claude on.
var errUnsupported = errors.New("no pseudo terminal on this system")

// Probe starts the interactive claude at path in dir on a pseudo terminal,
// reads its screen until it settles or something definite shows, and stops
// it. Nothing is typed into claude: it is ended before any conversation
// starts, so no session is saved.
//
// --strict-mcp-config keeps it from starting MCP servers, which reading the
// first screen does not need. The SessionStart hooks do run, as they do
// whenever claude starts.
//
// Ending ctx stops claude and returns at once.
func Probe(ctx context.Context, path, dir string, env []string) (Result, error) {
	t, err := start(path, dir, env)
	if errors.Is(err, errUnsupported) {
		return Result{Status: StatusUnsupported}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer t.stop()
	return watch(ctx, t)
}

// watch reads the screen from t until it can be classified.
func watch(ctx context.Context, t io.Reader) (Result, error) {
	var (
		mu      sync.Mutex
		raw     []byte
		changed = time.Now()
		closed  = make(chan struct{})
	)
	go func() {
		buf := make([]byte, 16<<10)
		for {
			n, err := t.Read(buf)
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
