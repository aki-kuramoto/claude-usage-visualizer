package i18n

import (
	"regexp"
	"testing"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		setting   settings.Language
		preferred []string
		want      Lang
	}{
		{settings.LanguageJapanese, []string{"en-US"}, Japanese},
		{settings.LanguageEnglish, []string{"ja-JP"}, English},
		{settings.LanguageSystem, []string{"ja-JP", "en-US"}, Japanese},
		{settings.LanguageSystem, []string{"fr-FR", "en-GB"}, English},
		{settings.LanguageSystem, []string{"en_US.UTF-8"}, English},
		{settings.LanguageSystem, []string{"fr-FR"}, Fallback},
		{settings.LanguageSystem, nil, Fallback},
	}
	for _, tt := range tests {
		if got := Resolve(tt.setting, tt.preferred); got != tt.want {
			t.Errorf("Resolve(%q, %v) = %q, want %q", tt.setting, tt.preferred, got, tt.want)
		}
	}
}

// Every language must have the same keys with the same placeholders, so that
// switching languages never leaves a hole in the window.
func TestTablesMatch(t *testing.T) {
	placeholder := regexp.MustCompile(`\{[a-z]+\}`)
	ref := table[Fallback]
	for lang, tab := range table {
		for key, text := range ref {
			other, ok := tab[key]
			if !ok {
				t.Errorf("%s has no %q", lang, key)
				continue
			}
			if a, b := placeholder.FindAllString(text, -1), placeholder.FindAllString(other, -1); len(a) != len(b) {
				t.Errorf("%s %q: placeholders %v, %s has %v", lang, key, b, Fallback, a)
			}
		}
		for key := range tab {
			if _, ok := ref[key]; !ok {
				t.Errorf("%s has %q, which %s lacks", lang, key, Fallback)
			}
		}
	}
}

// Every error kind the controller can report has a heading and an
// explanation. (The kinds are listed here rather than imported from package
// app, which imports this package.)
func TestErrorKindsHaveText(t *testing.T) {
	for _, kind := range []string{"not_logged_in", "not_found", "start", "timeout", "exited", "no_limits", "format", "other"} {
		for _, part := range []string{"_title", "_body"} {
			if _, ok := table[Fallback]["err_"+kind+part]; !ok {
				t.Errorf("no err_%s%s", kind, part)
			}
		}
	}
}

// Every theme has a name for the settings window.
func TestThemesHaveNames(t *testing.T) {
	for _, id := range theme.IDs() {
		if _, ok := table[Fallback]["theme_"+string(id)]; !ok {
			t.Errorf("no theme_%s", id)
		}
	}
}
