package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

func runHeadless(p paths.Paths, log *oplog.Log, s settings.Settings) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := newController(p, log, s, func(st app.State) {
		if st.Fetching {
			return
		}
		fmt.Fprintf(os.Stderr, "== %s (last success %s)", st.Status, st.LastSuccessAt)
		for _, l := range st.Limits {
			fmt.Fprintf(os.Stderr, " | %s %d%% resets %s", l.Label, l.UsedPct, l.ResetsAt)
		}
		if st.Error != "" {
			fmt.Fprintf(os.Stderr, " | error: %s", st.Error)
		}
		fmt.Fprintln(os.Stderr)
	})
	c.Run(ctx)
}
