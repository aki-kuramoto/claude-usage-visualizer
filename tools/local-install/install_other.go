//go:build !darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func defaultDir() (string, error) {
	return goInstallDir()
}

// install puts the bare executable in dir. There is no window on this OS yet;
// run it with -headless.
func install(root, dir string) error {
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	dest := filepath.Join(dir, "claude-usage-visualizer"+exe)
	tmp, err := os.CreateTemp(dir, ".claude-usage-visualizer-*"+exe)
	if err != nil {
		return err
	}
	tmp.Close()
	if err := goCmd(root, nil, "build", "-o", tmp.Name(), "./cmd/claude-usage-visualizer"); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := replace(tmp.Name(), dest); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	fmt.Println("==> installed", dest)
	fmt.Printf("Run it with: %q -headless\n", dest)
	return nil
}
