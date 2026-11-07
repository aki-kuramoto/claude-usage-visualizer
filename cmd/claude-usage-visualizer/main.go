// Command claude-usage-visualizer shows how much of the Claude Code plan
// limits has been used, and keeps a record of it.
//
// On macOS and Windows it opens a window. Elsewhere, until the GUI exists
// there, run it with -headless: it reads and records usage without a window,
// printing the operation log to standard error.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

func main() {
	headless := flag.Bool("headless", false, "read and record usage without a window")
	base := flag.String("base", "", "directory for settings and records (default <home>/.app-data/claude-usage-visualizer)")
	flag.Parse()
	if *headless {
		attachConsole()
	}

	p, err := paths.Default()
	if *base != "" {
		p, err = paths.Paths{Base: *base}, nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-usage-visualizer:", err)
		os.Exit(1)
	}

	// In headless mode the operation log is also printed, since there is
	// no window to look at.
	var echo io.Writer
	if *headless {
		echo = os.Stderr
	}
	log := oplog.New(p.Logs(), echo, os.Stderr)

	s, err := settings.Load(p.Settings())
	if err != nil {
		// Keep running with the defaults; the settings window shows them,
		// and saving from it replaces the broken file.
		log.Add(oplog.KindError, "", map[string]any{"what": "loading settings, using the defaults", "error": err.Error()})
		s = settings.Default()
	} else {
		log.Add(oplog.KindSettingsLoaded, "", map[string]any{"path": p.Settings(), "settings": s})
	}

	if *headless {
		runHeadless(p, log, s)
		return
	}
	if err := runGUI(p, log, s); err != nil {
		fmt.Fprintln(os.Stderr, "claude-usage-visualizer:", err)
		os.Exit(1)
	}
}

// newController is shared by both ways of running.
func newController(p paths.Paths, log *oplog.Log, s settings.Settings, publish func(app.State)) *app.Controller {
	return app.New(p, log, s, publish)
}
