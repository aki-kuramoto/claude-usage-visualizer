//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const executable = "claude-usage-visualizer.exe"

// defaultDir is ~/programs/claude-usage-visualizer.
func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "programs", "claude-usage-visualizer"), nil
}

// install builds the executable as a GUI program, which opens no console
// window when it starts, and puts it in dir. The icon and the manifest come
// from the .syso files that tools/genicon writes next to the main package.
func install(root, dir string) error {
	dest := filepath.Join(dir, executable)
	tmp, err := os.CreateTemp(dir, ".claude-usage-visualizer-*.exe")
	if err != nil {
		return err
	}
	tmp.Close()
	if err := goCmd(root, nil, "build", "-ldflags=-H=windowsgui", "-o", tmp.Name(), "./cmd/claude-usage-visualizer"); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	wasRunning := running(dest)
	if err := replace(tmp.Name(), dest); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	fmt.Println("==> installed", dest)
	if wasRunning {
		fmt.Println("The copy that is running is still the old one; quit it from the notification area and start the new one.")
	} else {
		fmt.Printf("Start it with: & '%s'\n", strings.ReplaceAll(dest, "'", "''"))
	}
	return nil
}

// running tells whether the executable at path is running.
func running(path string) bool {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-Process | Where-Object { $_.Path -eq '"+strings.ReplaceAll(path, "'", "''")+"' } | Measure-Object | Select-Object -ExpandProperty Count").Output()
	return err == nil && strings.TrimSpace(string(out)) != "0"
}
