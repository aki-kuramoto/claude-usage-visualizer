// Package oplog writes the operation log: what the application did and what
// happened to it, so that a person can tell whether it is working as
// intended and, when it is not, why.
//
// Each line is one event. Lines go to ops-YYYY-MM-DD.jsonl (UTC date) next to
// the usage record, and are never deleted by the application.
package oplog

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/jsonlog"
)

// Kind names an event.
type Kind string

const (
	KindAppStart       Kind = "app_start"
	KindAppStop        Kind = "app_stop"
	KindSettingsLoaded Kind = "settings_loaded"
	KindSettingsSaved  Kind = "settings_saved"
	KindClaudeFound    Kind = "claude_found"
	KindClaudeStart    Kind = "claude_start"
	KindClaudeInit     Kind = "claude_init" // claude reported its version
	KindClaudeKill     Kind = "claude_kill" // the application stopped claude
	KindClaudeExit     Kind = "claude_exit"
	KindFetchStart     Kind = "fetch_start"
	KindFetchEnd       Kind = "fetch_end"
	KindWindow         Kind = "window"       // the window was shown or closed
	KindPower          Kind = "power"        // the machine slept or woke
	KindAuth           Kind = "auth"         // claude's login was checked
	KindLoginOpened    Kind = "login_opened" // a terminal was opened to log in
	KindLoginExpiry    Kind = "login_expiry" // the interactive screen was read for the login's expiry
	KindTrustOpened    Kind = "trust_opened" // a terminal was opened to trust the working directory
	KindDaysCreated    Kind = "days_created" // a default days.setay was written
	KindDaysLoaded     Kind = "days_loaded"  // days.setay was read (again)
	KindDaysError      Kind = "days_error"   // days.setay could not be used
	KindDaysMoved      Kind = "days_moved"   // past entries were moved to past_days
	KindError          Kind = "error"        // anything else that went wrong
)

// Event is one line.
type Event struct {
	At   string `json:"at"`
	Kind Kind   `json:"kind"`
	// ID is the reading the event belongs to, the same value as the "id"
	// of its line in the usage record. Empty for events outside a reading.
	ID   string         `json:"id,omitempty"`
	Data map[string]any `json:"data,omitempty"`
}

// Log writes events. It is safe for concurrent use.
type Log struct {
	mu    sync.Mutex
	file  jsonlog.Daily
	echo  io.Writer // also print each line here, if not nil
	stray io.Writer // where to report a line that could not be written
}

// New returns a log that writes into dir. echo, if not nil, gets a copy of
// every line; stray gets the lines that could not be written to the file.
func New(dir string, echo, stray io.Writer) *Log {
	return &Log{file: jsonlog.Daily{Dir: dir, Prefix: "ops"}, echo: echo, stray: stray}
}

// Add writes one event now.
func (l *Log) Add(kind Kind, id string, data map[string]any) {
	l.AddAt(time.Now(), kind, id, data)
}

// AddAt writes one event with the given time.
func (l *Log) AddAt(at time.Time, kind Kind, id string, data map[string]any) {
	e := Event{At: at.Local().Format(time.RFC3339Nano), Kind: kind, ID: id, Data: data}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.echo != nil {
		if line, err := json.Marshal(e); err == nil {
			fmt.Fprintf(l.echo, "%s\n", line)
		}
	}
	if err := l.file.Append(at, e); err != nil && l.stray != nil {
		fmt.Fprintf(l.stray, "oplog: %v (event %s)\n", err, kind)
	}
}
