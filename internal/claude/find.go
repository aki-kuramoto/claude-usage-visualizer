package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Find returns the claude to run. configured, when not empty, is used as it
// is. Otherwise PATH is searched, then the places claude is usually installed:
// an application started from the Finder or the Start menu has a much
// shorter PATH than a shell.
func Find(configured string) (path string, tried []string, err error) {
	if configured != "" {
		if isExecutable(configured) {
			return configured, []string{configured}, nil
		}
		return "", []string{configured}, fmt.Errorf("the configured claude %s is not an executable file", configured)
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p, []string{"PATH"}, nil
	}
	tried = []string{"PATH"}
	for _, c := range candidates() {
		tried = append(tried, c)
		if isExecutable(c) {
			return c, tried, nil
		}
	}
	return "", tried, errors.New("claude was not found")
}

func candidates() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		// The native installer puts claude.exe in .local\bin; npm puts a
		// claude.cmd script in %APPDATA%\npm, which runs claude with node.
		c := []string{filepath.Join(home, ".local", "bin", "claude.exe")}
		if appData := os.Getenv("APPDATA"); appData != "" {
			c = append(c, filepath.Join(appData, "npm", "claude.cmd"))
		}
		return c
	}
	return []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, ".claude", "local", "claude"),
	}
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || st.Mode()&0o111 != 0
}

// Env returns the environment for claude, made from the application's own.
//
// Variables that mark a process as running inside Claude Code (CLAUDECODE and
// CLAUDE_CODE_*) are removed: when this application is started from a Claude
// Code session they would make claude think it is nested. PATH gets the
// directory of claude and a few usual directories, because hooks that claude
// runs at start may call commands that a short PATH does not reach.
func Env(parent []string, claudePath string) []string {
	var env []string
	pathValue := ""
	for _, kv := range parent {
		name, value, _ := strings.Cut(kv, "=")
		switch {
		case name == "CLAUDECODE", strings.HasPrefix(name, "CLAUDE_CODE_"):
			continue
		case strings.EqualFold(name, "PATH"):
			pathValue = value
			continue
		}
		env = append(env, kv)
	}
	dirs := filepath.SplitList(pathValue)
	have := map[string]bool{}
	for _, d := range dirs {
		have[d] = true
	}
	extra := []string{filepath.Dir(claudePath)}
	if runtime.GOOS != "windows" {
		home, _ := os.UserHomeDir()
		extra = append(extra, "/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin"), "/usr/bin", "/bin")
	}
	for _, d := range extra {
		if d != "" && !have[d] {
			dirs = append(dirs, d)
			have[d] = true
		}
	}
	return append(env, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
}
