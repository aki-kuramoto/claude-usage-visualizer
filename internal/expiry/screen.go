// Package expiry finds out how soon Claude Code's login expires.
//
// Claude Code says so only on its interactive screen: when the login is
// close to expiring it shows "Your login expires in 3 days · run /login to
// renew" right after starting. Neither "claude auth status" nor the -p mode
// this application otherwise uses reports it. So this package starts the
// interactive claude on a pseudo terminal, reads what it draws for a few
// seconds, and stops it.
//
// The screen is read as a stream of text with the escape sequences turned
// into spaces, not by emulating a terminal: claude separates words with
// cursor movements, and turning those into spaces is enough to find the few
// phrases needed here. This is cruder than a real terminal emulator, but it
// needs nothing beyond the pseudo terminal itself.
package expiry

import (
	"regexp"
	"strconv"
	"strings"
)

// Status is what the screen said.
type Status string

const (
	// StatusDays: the login expires in DaysLeft days.
	StatusDays Status = "days"
	// StatusNone: claude started normally and showed no expiry warning.
	// Claude Code shows it only when the expiry is near.
	StatusNone Status = "none"
	// StatusTrustNeeded: claude asked whether to trust its working directory
	// before anything else. The user has to answer that once.
	StatusTrustNeeded Status = "trust_needed"
	// StatusLoggedOut: claude is not logged in.
	StatusLoggedOut Status = "logged_out"
	// StatusUnknown: the screen did not settle into anything recognisable.
	StatusUnknown Status = "unknown"
	// StatusUnsupported: this OS has no pseudo terminal support here yet.
	StatusUnsupported Status = "unsupported"
)

// Result is one reading of the screen.
type Result struct {
	Status   Status `json:"status"`
	DaysLeft int    `json:"days_left,omitempty"`
	// Excerpt is the end of the screen text, for the operation log when the
	// screen was not understood.
	Excerpt string `json:"excerpt,omitempty"`
}

var (
	// escapes matches the escape sequences claude prints: CSI (cursor
	// movement, colours, modes), OSC (titles, queries), DCS, and the short
	// two-character ones.
	escapes = regexp.MustCompile(`\x1b\[[0-9;?<>=:]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1bP[^\x1b]*\x1b\\|\x1b[()][0-9A-Za-z]|\x1b[@-Z\\-_78=>]`)
	spaces  = regexp.MustCompile(`\s+`)

	expires   = regexp.MustCompile(`Your login expires in (\d+) days?`)
	trust     = regexp.MustCompile(`Quick safety check|trust this folder|Do you trust the files`)
	loggedOut = regexp.MustCompile(`Not logged in`)
	// ready is the input prompt, drawn once claude is ready to be typed to.
	ready = regexp.MustCompile(`❯`)
)

// Text turns raw terminal output into plain text, with every escape
// sequence and run of white space made a single space.
func Text(raw []byte) string {
	s := escapes.ReplaceAllString(string(raw), " ")
	s = strings.ReplaceAll(s, "\x07", " ")
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

// Classify reads the screen. settled says whether claude has stopped
// drawing; until then, only a definite finding (a warning, the trust dialog)
// is reported, and otherwise StatusUnknown.
func Classify(raw []byte, settled bool) Result {
	text := Text(raw)
	if m := expires.FindStringSubmatch(text); m != nil {
		days, _ := strconv.Atoi(m[1])
		return Result{Status: StatusDays, DaysLeft: days}
	}
	if trust.MatchString(text) {
		return Result{Status: StatusTrustNeeded}
	}
	if !settled {
		return Result{Status: StatusUnknown}
	}
	if loggedOut.MatchString(text) {
		return Result{Status: StatusLoggedOut}
	}
	if ready.MatchString(text) {
		return Result{Status: StatusNone}
	}
	return Result{Status: StatusUnknown, Excerpt: excerpt(text)}
}

func excerpt(text string) string {
	const max = 600
	r := []rune(text)
	if len(r) > max {
		r = r[len(r)-max:]
	}
	return string(r)
}
