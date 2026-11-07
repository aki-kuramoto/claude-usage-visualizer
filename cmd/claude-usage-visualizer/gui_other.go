//go:build !darwin && !windows

package main

import (
	"errors"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

func runGUI(paths.Paths, *oplog.Log, settings.Settings) error {
	return errors.New("there is no window on this OS yet; run with -headless")
}
