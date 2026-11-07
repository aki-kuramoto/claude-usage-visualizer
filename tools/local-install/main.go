// local-install builds claude-usage-visualizer from this working tree and
// installs it for daily use.
//
//	go run ./tools/local-install            vet, test, build, install
//	go run ./tools/local-install -q         skip vet and test
//	go run ./tools/local-install -dir DIR   install into DIR instead
//
// On macOS it builds ClaudeUsageVisualizer.app into ~/Applications. Elsewhere,
// where there is no window yet, it installs the bare executable (to be run
// with -headless) where `go install` would put it.
//
// The new copy is built next to the destination under a temporary name and
// then moved into place, so a copy that is running keeps working until it is
// quit (see install_*.go).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	quick := flag.Bool("q", false, "skip go vet and go test")
	dir := flag.String("dir", "", "install directory (default: ~/Applications on macOS, else where go install puts binaries)")
	flag.Parse()
	if err := run(*quick, *dir); err != nil {
		fmt.Fprintln(os.Stderr, "local-install:", err)
		os.Exit(1)
	}
}

func run(quick bool, dir string) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	if !quick {
		if err := goCmd(root, nil, "vet", "./..."); err != nil {
			return err
		}
		if err := goCmd(root, nil, "test", "./..."); err != nil {
			return err
		}
	}
	if dir == "" {
		if dir, err = defaultDir(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return install(root, dir)
}

// goCmd runs the go command in dir, with extra environment variables.
func goCmd(dir string, env []string, args ...string) error {
	fmt.Println("==> go", strings.Join(args, " "))
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func goEnv(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func moduleRoot() (string, error) {
	mod, err := goEnv("GOMOD")
	if err != nil {
		return "", err
	}
	if mod == "" || mod == os.DevNull {
		return "", fmt.Errorf("run it inside the claude-usage-visualizer repository")
	}
	return filepath.Dir(mod), nil
}

// goInstallDir is where `go install` would put a binary.
func goInstallDir() (string, error) {
	if bin, err := goEnv("GOBIN"); err != nil || bin != "" {
		return bin, err
	}
	gopath, err := goEnv("GOPATH")
	if err != nil {
		return "", err
	}
	// GOPATH can be a list; go install uses the first entry.
	return filepath.Join(filepath.SplitList(gopath)[0], "bin"), nil
}
