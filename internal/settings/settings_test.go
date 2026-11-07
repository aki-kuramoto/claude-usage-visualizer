package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	setay "github.com/aki-kuramoto/setay-go"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMissingFileGivesDefaults(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "settings.setay"))
	if err != nil || s != Default() {
		t.Errorf("got %+v, %v", s, err)
	}
}

func TestSaveCreatesCommentedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.setay")
	want := Default()
	want.IntervalMinutes = 3
	want.ClaudePath = `C:\Users\o'neil\claude.exe`
	want.AlwaysOnTop = true
	want.Language = LanguageEnglish
	want.CloseAction = CloseQuit
	want.Theme = theme.System7
	res, err := Save(path, want)
	if err != nil || !res.Created || len(res.Changed) != len(fields) {
		t.Fatalf("got %+v, %v", res, err)
	}
	body := read(t, path)
	if !strings.Contains(body, "# How often usage is read") || !strings.Contains(body, "interval_minutes = 3;") {
		t.Errorf("new file:\n%s", body)
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestSaveKeepsCommentsAndUnchangedText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.setay")
	original := "# mine\n{\n\t# check often\n\tinterval_minutes = 5; # five\n\n\t# from my home\n\tclaude_path = \"${HOME}/bin/claude\";\n" +
		"\talways_on_top;\n\tlanguage = 'system';\n\tclose_action = 'ask';\n\tlogin_expiry_check = true;\n\tstart_compact = false;\n\ttheme = 'platinum';\n}\n"
	write(t, path, original)
	t.Setenv("HOME", "/home/me")

	s, err := Load(path)
	if err != nil || s.ClaudePath != "/home/me/bin/claude" || !s.AlwaysOnTop {
		t.Fatalf("got %+v, %v", s, err)
	}

	// Saving what was loaded changes nothing at all.
	res, err := Save(path, s)
	if err != nil || len(res.Changed) != 0 || read(t, path) != original {
		t.Errorf("unchanged save: %+v, %v\n%s", res, err, read(t, path))
	}

	// Changing the interval rewrites only that value.
	s.IntervalMinutes = 10
	res, err = Save(path, s)
	if err != nil || strings.Join(res.Changed, ",") != "interval_minutes" {
		t.Fatalf("got %+v, %v", res, err)
	}
	want := strings.Replace(original, "interval_minutes = 5;", "interval_minutes = 10;", 1)
	if got := read(t, path); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveAppendsMissingEntries(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"partial": "{\n\tinterval_minutes = 5;\n}\n",
		"empty":   "# nothing yet\n{}\n",
	} {
		path := filepath.Join(dir, name+".setay")
		write(t, path, body)
		want := Default()
		want.ClaudePath = "/opt/claude"
		want.CloseAction = CloseWindow
		if _, err := Save(path, want); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got, err := Load(path)
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v\n%s", name, got, err, read(t, path))
		}
		if name == "empty" && !strings.HasPrefix(read(t, path), "# nothing yet\n") {
			t.Errorf("comment lost:\n%s", read(t, path))
		}
	}
}

func TestSaveMovesBrokenFileAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.setay")
	write(t, path, "{ interval_minutes = ")
	res, err := Save(path, Default())
	if err != nil || !res.Created || res.Backup == "" {
		t.Fatalf("got %+v, %v", res, err)
	}
	if read(t, res.Backup) != "{ interval_minutes = " {
		t.Error("the broken file was not kept")
	}
	if got, err := Load(path); err != nil || got != Default() {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestLoadRejects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.setay")
	for _, body := range []string{
		"{ interval_minutes = 0; }",
		"{ interval_minutes = 61; }",
		"{ interval = 3; }", // unknown key, most likely a typo
		"{ language = 'fr'; }",
		"{ close_action = 'minimize'; }",
		"{ theme = 'aqua'; }",
		"{ not setay",
	} {
		write(t, path, body)
		if _, err := Load(path); err == nil {
			t.Errorf("%s accepted", body)
		}
	}
	if _, err := Save(path, Settings{IntervalMinutes: 0}); err == nil {
		t.Error("saved an interval of 0")
	}
}

func TestValueText(t *testing.T) {
	for _, v := range []any{"it's \"q\" ${HOME} \\ \t", "", `C:\x\y`} {
		text, err := valueText(v)
		if err != nil {
			t.Fatal(err)
		}
		var back struct {
			V string `setay:"v"`
		}
		if err := setay.Unmarshal([]byte("{ v = "+text+"; }"), &back); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		if back.V != v {
			t.Errorf("%q came back as %q (text %s)", v, back.V, text)
		}
	}
}

// A file written before always_on_top, language and close_action existed
// reads with their defaults, and saving adds them without touching the rest.
func TestOlderFileGetsNewEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.setay")
	original := "# old\n{\n\tinterval_minutes = 1;\n\tclaude_path = '';\n}\n"
	write(t, path, original)
	s, err := Load(path)
	want := Default()
	want.IntervalMinutes = 1
	if err != nil || s != want {
		t.Fatalf("got %+v, %v", s, err)
	}
	res, err := Save(path, s)
	if err != nil || strings.Join(res.Changed, ",") != "always_on_top,language,close_action,login_expiry_check,start_compact,theme" {
		t.Fatalf("got %+v, %v", res, err)
	}
	body := read(t, path)
	if !strings.HasPrefix(body, "# old\n{\n\tinterval_minutes = 1;\n\tclaude_path = '';\n") ||
		!strings.Contains(body, "always_on_top = false;") || !strings.Contains(body, "close_action = 'ask';") {
		t.Errorf("got\n%s", body)
	}
}
