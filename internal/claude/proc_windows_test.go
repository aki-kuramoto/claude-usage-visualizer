//go:build windows

package claude

import "testing"

// claude installed by npm is a .cmd script: cmd.exe runs it and starts the
// real program as its child. Both must be measured and ended together.
func TestThroughCmdScript(t *testing.T) {
	r, logs := setupWith(t, "ok", true)
	testAnswersRepeatedly(t, r, logs)
}

func TestTimeoutKillsChildOfCmdScript(t *testing.T) {
	r, logs := setupWith(t, "hang", true)
	testTimeoutKillsAndRestarts(t, r, logs)
}
