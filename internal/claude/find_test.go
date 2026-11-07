package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvDropsNestingMarkersAndExtendsPath(t *testing.T) {
	parent := []string{
		"HOME=/h", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=x", "CLAUDE_CONFIG_DIR=/c",
		"PATH=" + strings.Join([]string{"/usr/bin", "/bin"}, string(os.PathListSeparator)),
	}
	env := Env(parent, filepath.Join("/opt", "claude", "claude"))
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "CLAUDECODE") || strings.Contains(joined, "CLAUDE_CODE_ENTRYPOINT") {
		t.Errorf("nesting markers kept: %v", env)
	}
	if !strings.Contains(joined, "CLAUDE_CONFIG_DIR=/c") || !strings.Contains(joined, "HOME=/h") {
		t.Errorf("other variables lost: %v", env)
	}
	var path string
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = strings.TrimPrefix(kv, "PATH=")
		}
	}
	dirs := filepath.SplitList(path)
	if dirs[0] != "/usr/bin" || !strings.Contains(path, filepath.Join("/opt", "claude")) {
		t.Errorf("PATH = %q", path)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			t.Errorf("%s twice in PATH %q", d, path)
		}
		seen[d] = true
	}
}

func TestFindConfigured(t *testing.T) {
	if _, _, err := Find(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing configured claude was accepted")
	}
}
