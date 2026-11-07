// Package settings reads and writes the settings the user can change.
//
// The file is in the setay format (github.com/aki-kuramoto/setay-go). It is
// meant to be edited by hand as well as from the settings window, so saving
// from the window changes only the values that differ and leaves everything
// else -- comments, layout, ${VAR} references in unchanged values -- as it
// was written.
package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	setay "github.com/aki-kuramoto/setay-go"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
)

const (
	MinIntervalMinutes     = 1
	MaxIntervalMinutes     = 60
	DefaultIntervalMinutes = 5
)

// Language is the language of the window's text.
type Language string

const (
	// LanguageSystem follows the language set for the OS.
	LanguageSystem   Language = "system"
	LanguageJapanese Language = "ja"
	LanguageEnglish  Language = "en"
)

// CloseAction is what closing the window does. On macOS and Windows the
// application can keep running without its window (in the Dock, in the
// notification area); elsewhere closing the window always ends it.
type CloseAction string

const (
	// CloseAsk asks whether to quit or only close the window.
	CloseAsk CloseAction = "ask"
	// CloseWindow closes the window and keeps recording.
	CloseWindow CloseAction = "close"
	// CloseQuit quits the application.
	CloseQuit CloseAction = "quit"
)

// Settings is the content of the settings file.
type Settings struct {
	// IntervalMinutes is how often usage is read.
	IntervalMinutes int `setay:"interval_minutes" json:"interval_minutes"`
	// ClaudePath is where the claude command is. Empty means find it.
	ClaudePath string `setay:"claude_path" json:"claude_path"`
	// AlwaysOnTop keeps the window above other applications' windows.
	AlwaysOnTop bool `setay:"always_on_top" json:"always_on_top"`
	// Language chooses the language of the window's text.
	Language Language `setay:"language" json:"language"`
	// CloseAction is what closing the window does on macOS and Windows.
	CloseAction CloseAction `setay:"close_action" json:"close_action"`
	// LoginExpiryCheck has the application start the interactive claude
	// briefly, about once a day, to read whether the login expires soon.
	LoginExpiryCheck bool `setay:"login_expiry_check" json:"login_expiry_check"`
	// StartCompact starts the window in the compact view (one line of text
	// instead of the bars). The view can be switched while running.
	StartCompact bool `setay:"start_compact" json:"start_compact"`
	// Theme chooses the look of the window (see package theme).
	Theme theme.ID `setay:"theme" json:"theme"`
}

// fields lists the settings in file order, with what the new-file template
// says about each.
var fields = []struct {
	key     string
	comment string
	value   func(Settings) any
}{
	{"interval_minutes", "How often usage is read, in minutes: 1 to 60.",
		func(s Settings) any { return s.IntervalMinutes }},
	{"claude_path", "Where the claude command is. Empty means look in PATH and the usual places. Environment variables such as ${HOME} can be used.",
		func(s Settings) any { return s.ClaudePath }},
	{"always_on_top", "true keeps the window above the windows of other applications.",
		func(s Settings) any { return s.AlwaysOnTop }},
	{"language", "The language of the window: 'system' (as the OS is set), 'ja' (Japanese) or 'en' (English).",
		func(s Settings) any { return string(s.Language) }},
	{"close_action", "What closing the window does on macOS and Windows: 'ask' (ask whether to quit), 'close' (close it and keep recording) or 'quit' (quit without asking).",
		func(s Settings) any { return string(s.CloseAction) }},
	{"login_expiry_check", "true checks when the login expires (about once a day, by starting the interactive claude for a few seconds and reading whether it warns of the expiry).",
		func(s Settings) any { return s.LoginExpiryCheck }},
	{"start_compact", "true starts in the compact view (the title bar alone, with the usage in the title). While running, the box on the right of the title bar, or a double-click on the title bar, switches the view.",
		func(s Settings) any { return s.StartCompact }},
	{"theme", "The look of the window: 'platinum' (in the style of Mac OS 8) or 'system7' (in the style of System 7).",
		func(s Settings) any { return string(s.Theme) }},
}

// Default returns the settings used when there is no file.
func Default() Settings {
	return Settings{
		IntervalMinutes: DefaultIntervalMinutes,
		Language:        LanguageSystem,
		CloseAction:     CloseAsk,
		// On unless turned off: see README.md for what it does and why.
		LoginExpiryCheck: true,
		Theme:            theme.Default,
	}
}

// Interval returns IntervalMinutes as a duration.
func (s Settings) Interval() time.Duration {
	return time.Duration(s.IntervalMinutes) * time.Minute
}

// Validate reports a setting that is out of range.
func (s Settings) Validate() error {
	if s.IntervalMinutes < MinIntervalMinutes || s.IntervalMinutes > MaxIntervalMinutes {
		return fmt.Errorf("interval must be %d to %d minutes, not %d",
			MinIntervalMinutes, MaxIntervalMinutes, s.IntervalMinutes)
	}
	switch s.Language {
	case LanguageSystem, LanguageJapanese, LanguageEnglish:
	default:
		return fmt.Errorf("language must be %q, %q or %q, not %q",
			LanguageSystem, LanguageJapanese, LanguageEnglish, s.Language)
	}
	switch s.CloseAction {
	case CloseAsk, CloseWindow, CloseQuit:
	default:
		return fmt.Errorf("close_action must be %q, %q or %q, not %q",
			CloseAsk, CloseWindow, CloseQuit, s.CloseAction)
	}
	if _, ok := theme.Get(s.Theme); !ok {
		return fmt.Errorf("theme must be one of %q, not %q", theme.IDs(), s.Theme)
	}
	return nil
}

// Load reads the file at path. A missing file gives the defaults. A file that
// cannot be read, has a key this version does not know, or holds a value out
// of range is an error, so that a typo is not silently replaced by a default.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	s, err := decode(data)
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func decode(data []byte) (Settings, error) {
	s := Default()
	if err := setay.Unmarshal(data, &s, setay.DisallowUnknownFields()); err != nil {
		return Settings{}, err
	}
	if err := s.Validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveResult says what Save did beyond writing the values.
type SaveResult struct {
	// Created is true when there was no usable file and a new one was
	// written.
	Created bool
	// Backup is where an unreadable file was moved before a new one was
	// written in its place, or empty.
	Backup string
	// Changed lists the keys whose values were written.
	Changed []string
}

// Save writes s to path.
//
// An existing file is edited in place: only the entries whose values differ
// from s are rewritten, and missing entries are appended, so comments and
// formatting survive. When there is no file, a new one with a comment for
// each entry is written. A file that cannot be parsed or decoded is moved
// aside (to <path>.broken-<time>) and replaced by a new one, since the
// settings window has no other way to recover from it.
func Save(path string, s Settings) (SaveResult, error) {
	var res SaveResult
	if err := s.Validate(); err != nil {
		return res, err
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		res.Created = true
	case err != nil:
		return res, err
	}

	var out string
	if !res.Created {
		out, res.Changed, err = edit(string(data), s)
		if err != nil {
			res.Backup = fmt.Sprintf("%s.broken-%s", path, time.Now().Format("20060102-150405"))
			if rerr := os.Rename(path, res.Backup); rerr != nil {
				return res, fmt.Errorf("%v; moving it aside also failed: %w", err, rerr)
			}
			res.Created = true
		}
	}
	if res.Created {
		if out, err = template(s); err != nil {
			return res, err
		}
		res.Changed = nil
		for _, f := range fields {
			res.Changed = append(res.Changed, f.key)
		}
	}

	// Check that what is about to be written reads back as s.
	back, err := decode([]byte(out))
	if err != nil {
		return res, fmt.Errorf("the edited settings do not read back: %w", err)
	}
	if back != s {
		return res, fmt.Errorf("the edited settings read back as %+v, not %+v", back, s)
	}
	return res, writeAtomically(path, []byte(out))
}

// edit applies s to the existing text src, touching only what differs.
func edit(src string, s Settings) (string, []string, error) {
	current, err := decode([]byte(src))
	if err != nil {
		return "", nil, err
	}
	doc, err := setay.ParseDocument(src)
	if err != nil {
		return "", nil, err
	}
	cs := doc.Changes()
	var changed []string
	for _, f := range fields {
		want := f.value(s)
		node, ok := doc.Field(f.key)
		if ok && f.value(current) == want {
			continue // unchanged; keep it exactly as written
		}
		text, err := valueText(want)
		if err != nil {
			return "", nil, err
		}
		if ok {
			err = cs.SetRaw(node, text)
		} else {
			err = appendEntry(doc, cs, f.key, text)
		}
		if err != nil {
			return "", nil, err
		}
		changed = append(changed, f.key)
	}
	if cs.Len() == 0 {
		return src, nil, nil
	}
	edited, err := cs.Apply()
	if err != nil {
		return "", nil, err
	}
	return edited.String(), changed, nil
}

// appendEntry adds key = text at the end of the top-level dict.
//
// setay-go's AppendEntry copies the layout of the existing entries, and so
// refuses a dict that has none; an empty dict gets the entry through
// AppendRaw instead, with the layout written out here.
func appendEntry(doc *setay.Document, cs *setay.ChangeSet, key, text string) error {
	err := cs.AppendEntry(doc.Root(), key, text)
	if err == nil || !strings.Contains(err.Error(), "empty dict") {
		return err
	}
	return cs.AppendRaw(doc.Root(), fmt.Sprintf("\n\t%s = %s;\n", key, text))
}

// template is the text of a new settings file.
func template(s Settings) (string, error) {
	var b strings.Builder
	b.WriteString("# Settings of claude-usage-visualizer.\n")
	b.WriteString("# You may edit this file by hand. Saving from the settings window rewrites only the values that changed.\n")
	b.WriteString("{\n")
	for i, f := range fields {
		text, err := valueText(f.value(s))
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "\t# %s\n\t%s = %s;\n", f.comment, f.key, text)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// valueText turns one Go value into setay text.
//
// setay-go's Marshal only takes a struct or a map, so the value is marshaled
// inside a one-entry map and its text is read back out of the result. This
// leaves quoting and escaping to the library.
func valueText(v any) (string, error) {
	data, err := setay.Marshal(map[string]any{"v": v})
	if err != nil {
		return "", err
	}
	doc, err := setay.ParseDocument(string(data))
	if err != nil {
		return "", err
	}
	node, ok := doc.Field("v")
	if !ok {
		return "", fmt.Errorf("setay: no value in %q", data)
	}
	return node.Raw(), nil
}

// writeAtomically writes through a temporary file so that a crash cannot
// leave half a file behind.
func writeAtomically(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
