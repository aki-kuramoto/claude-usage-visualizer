package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Auth is the part of "claude auth status --json" this application uses.
// The e-mail address and organization it also reports are left out on
// purpose: they are not needed, and are better kept out of the logs.
type Auth struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	SubscriptionType string `json:"subscriptionType,omitempty"`
}

// AuthStatus runs "claude auth status --json". It takes a fraction of a
// second, so it can be asked often. claude exits with 1 when not logged in,
// which is not an error here: the JSON says so.
func AuthStatus(path, dir string, env []string) (Auth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "auth", "status", "--json")
	cmd.Dir = dir
	// On Windows, without this a console window would flash up each time.
	prepare(cmd)
	cmd.Env = env
	out, err := cmd.Output()
	var a Auth
	if jerr := json.Unmarshal(out, &a); jerr != nil {
		if err != nil {
			return Auth{}, fmt.Errorf("claude auth status: %v", err)
		}
		return Auth{}, fmt.Errorf("claude auth status printed something that is not JSON: %v", jerr)
	}
	return a, nil
}
