package expiry

import (
	"os"
	"testing"
)

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The captures in testdata were taken from Claude Code 2.1.300 on
// 2026-10-17, with the home directory and the scratch path replaced.

func TestTrustDialog(t *testing.T) {
	raw := mustRead(t, "testdata/trust-dialog.bin")
	if r := Classify(raw, false); r.Status != StatusTrustNeeded {
		t.Errorf("got %+v", r)
	}
}

func TestLoggedOut(t *testing.T) {
	raw := mustRead(t, "testdata/logged-out.bin")
	if r := Classify(raw, false); r.Status != StatusUnknown {
		t.Errorf("before settling: %+v", r)
	}
	if r := Classify(raw, true); r.Status != StatusLoggedOut {
		t.Errorf("got %+v", r)
	}
}

// The warning as it looked on 2026-10-15, drawn the way claude draws words:
// with cursor movements between them.
func TestExpiryWarning(t *testing.T) {
	raw := []byte("\x1b[2J\x1b[H ▐▛███▛█   Claude Code v2.1.278\r\n" +
		"\x1b[33m⚠\x1b[1CYour\x1b[1Clogin\x1b[1Cexpires\x1b[1Cin\x1b[1C3\x1b[1Cdays\x1b[39m\x1b[2m\x1b[1C·\x1b[1Crun\x1b[1C/login\x1b[1Cto\x1b[1Crenew\x1b[22m\r\n" +
		"\x1b[38;5;246m❯\x1b[39m \r\n")
	r := Classify(raw, false)
	if r.Status != StatusDays || r.DaysLeft != 3 {
		t.Errorf("got %+v", r)
	}
	one := []byte("Your login expires in 1 day · run /login to renew\r\n❯")
	if r := Classify(one, true); r.Status != StatusDays || r.DaysLeft != 1 {
		t.Errorf("one day: %+v", r)
	}
}

func TestNoWarning(t *testing.T) {
	raw := []byte("\x1b[H Claude Code v2.1.300\r\n\x1b[1C❯\x1b[1CTry \"edit <filepath> to...\"\r\n")
	if r := Classify(raw, true); r.Status != StatusNone {
		t.Errorf("got %+v", r)
	}
	if r := Classify([]byte("loading..."), true); r.Status != StatusUnknown || r.Excerpt != "loading..." {
		t.Errorf("unrecognised screen: %+v", r)
	}
}

func TestText(t *testing.T) {
	got := Text([]byte("a\x1b[1Cb\x1b]0;title\x07c\x1b[?2026h  d\r\ne"))
	if got != "a b c d e" {
		t.Errorf("got %q", got)
	}
}
