// Package i18n holds the text the window shows, in each language the
// application speaks, and decides which language to use.
//
// The text lives here rather than in the GUI code so that the windows of every
// OS share it. Strings may hold placeholders in braces ({time}, {app},
// {path}, {error}) that the GUI fills in.
package i18n

import (
	"strings"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
)

// Lang is a language the application has text for.
type Lang string

const (
	Japanese Lang = "ja"
	English  Lang = "en"
)

// Fallback is used when neither the setting nor the OS names a language the
// application has.
const Fallback = English

// Resolve picks the language: the setting when it names one, otherwise the
// first of the OS's preferred languages (most preferred first, such as
// "ja-JP" or "en_US.UTF-8") that the application has.
func Resolve(setting settings.Language, preferred []string) Lang {
	switch setting {
	case settings.LanguageJapanese:
		return Japanese
	case settings.LanguageEnglish:
		return English
	}
	for _, p := range preferred {
		p = strings.ToLower(p)
		switch {
		case strings.HasPrefix(p, "ja"):
			return Japanese
		case strings.HasPrefix(p, "en"):
			return English
		}
	}
	return Fallback
}

// DateLocale is the locale for formatting dates and times in lang.
func DateLocale(lang Lang) string {
	if lang == Japanese {
		return "ja_JP"
	}
	return "en_US"
}

// Strings returns the text for lang, keyed by name.
func Strings(lang Lang) map[string]string {
	if t, ok := table[lang]; ok {
		return t
	}
	return table[Fallback]
}

var table = map[Lang]map[string]string{
	Japanese: {
		"window_title":    "Claude Code の使用量",
		"pending":         "まだ取得できていません",
		"fetching_suffix": " (取得中…)",
		"failed_never":    "まだ一度も取得できていません。",
		"stale_note":      "灰色のバーは {time} に取得した古い値です。",
		"read_at":         "{time} に取得",
		"resets_at":       "リセット {time}",
		"no_reset":        "リセット時刻なし",
		"remaining_dh":    "あと {d}日{h}時間",
		"remaining_hm":    "あと {h}時間{m}分",
		"remaining_m":     "あと {m}分",
		"elapsed":         "時間の経過 {pct}%",
		"elapsed_days":    "使う日で {pct}%",
		"elapsed_ahead":   "残りの日で {pct}%",
		"days_error":      "日の設定 (days.setay) を読めません: {error}",
		"days_file":       "日の設定: {path}",

		"menu_about":      "{app} について",
		"menu_settings":   "設定…",
		"menu_hide":       "{app} を隠す",
		"menu_quit":       "{app} を終了",
		"menu_edit":       "編集",
		"menu_undo":       "取り消す",
		"menu_cut":        "カット",
		"menu_copy":       "コピー",
		"menu_paste":      "ペースト",
		"menu_select_all": "すべてを選択",
		"menu_usage":      "使用量",
		"menu_fetch_now":  "今すぐ取得",
		"menu_show":       "使用量のウィンドウを表示",
		"menu_window":     "ウィンドウ",
		"menu_minimize":   "しまう",
		"menu_close":      "閉じる",

		"err_not_logged_in_title": "Claude Code にログインしていません",
		"err_not_logged_in_body":  "ログインすると、自動で取得を再開します。",
		"err_not_found_title":     "claude が見つかりません",
		"err_not_found_body":      "設定で claude の場所を指定してください。",
		"err_start_title":         "claude を起動できません",
		"err_start_body":          "設定の claude の場所と、そのファイルを実行できるかを確かめてください。",
		"err_timeout_title":       "claude が応答しません",
		"err_timeout_body":        "次の取得で claude を起動し直します。",
		"err_exited_title":        "claude が途中で終了しました",
		"err_exited_body":         "次の取得で claude を起動し直します。",
		"err_no_limits_title":     "/usage に使用量が出ていません",
		"err_no_limits_body":      "ログインはしています。次の取得で claude を起動し直します。",
		"err_format_title":        "/usage の出力を読めません",
		"err_format_body":         "Claude Code の更新で出力の形が変わった可能性があります。",
		"err_other_title":         "取得に失敗しました",
		"err_other_body":          "",
		"login_button":            "ログインする…",
		"open_settings_button":    "設定を開く…",
		"login_failed":            "ターミナルを開けませんでした: {error}",
		"notify_logged_out_title": "Claude Code にログインしていません",
		"notify_logged_out_body":  "使用量を取得できません。ログインすると自動で再開します。",
		"expiry_days":             "ログインの期限まであと {days} 日です",
		"expiry_trust":            "ログインの期限を確かめるには、作業フォルダーを一度だけ信頼する必要があります。",
		"trust_button":            "フォルダーを信頼する…",
		"trust_failed":            "ターミナルを開けませんでした: {error}",
		"trust_script_message":    "claude-usage-visualizer: ログインの期限を確かめるために、この作業フォルダーで claude を起動します。信頼の確認が出たら「Yes, I trust this folder」を選び、claude が起動したら /exit で終了してください。このフォルダーは claude-usage-visualizer 専用で、中身は空です。",
		"login_expiry_check":      "ログインの期限を確かめる (1 日 1 回ほど、対話モードの claude を数秒だけ起動)",
		"compact_session":         "5h",
		"compact_week":            "週",
		"compact_pending":         "まだ取得できていません",
		"compact_failed":          "取得できていません",
		"menu_compact":            "小さく表示",
		"start_compact":           "起動したときは小さく表示する",
		"menu_view":               "表示",
		"menu_settings_menu":      "設定",
		"theme_label":             "見た目:",
		"theme_platinum":          "プラチナ (Mac OS 8 風)",
		"theme_system7":           "System 7 風",
		"settings_title":          "設定",
		"interval_label":          "取得間隔:",
		"interval_unit":           "分 (1〜60)",
		"claude_path_label":       "claude の場所:",
		"claude_path_hint":        "空なら自動で探します",
		"always_on_top":           "常にほかのウィンドウより手前に表示する",
		"language_label":          "言語:",
		"language_system":         "OS の設定に従う",
		"close_label":             "ウィンドウを閉じたとき:",
		"close_ask":               "終了するか訊く",
		"close_close":             "訊かずに閉じる (記録は続ける)",
		"close_quit":              "訊かずに終了する",
		"settings_file":           "設定ファイル: {path}",
		"logs_dir":                "記録の場所: {path}",
		"cancel":                  "キャンセル",
		"save":                    "保存",
		"save_failed":             "保存できませんでした: {error}",
		"close_alert_title":       "アプリも終了しますか?",
		"close_alert_message":     "ウィンドウを閉じるだけなら、アプリは動き続けて使用量を記録します。",
		"close_alert_quit":        "終了する",
		"close_alert_close":       "閉じるだけ",
		"close_alert_cancel":      "キャンセル",
	},
	English: {
		"window_title":    "Claude Code usage",
		"pending":         "Not read yet",
		"fetching_suffix": " (reading…)",
		"failed_never":    "Nothing has been read yet.",
		"stale_note":      "The gray bars are old values, read at {time}.",
		"read_at":         "Read at {time}",
		"resets_at":       "Resets {time}",
		"no_reset":        "No reset time",
		"remaining_dh":    "{d}d {h}h left",
		"remaining_hm":    "{h}h {m}m left",
		"remaining_m":     "{m}m left",
		"elapsed":         "{pct}% of the time elapsed",
		"elapsed_days":    "{pct}% by usage days",
		"elapsed_ahead":   "{pct}% by the days ahead",
		"days_error":      "Can't use the days file (days.setay): {error}",
		"days_file":       "Days file: {path}",

		"menu_about":      "About {app}",
		"menu_settings":   "Settings…",
		"menu_hide":       "Hide {app}",
		"menu_quit":       "Quit {app}",
		"menu_edit":       "Edit",
		"menu_undo":       "Undo",
		"menu_cut":        "Cut",
		"menu_copy":       "Copy",
		"menu_paste":      "Paste",
		"menu_select_all": "Select All",
		"menu_usage":      "Usage",
		"menu_fetch_now":  "Read Now",
		"menu_show":       "Show Usage Window",
		"menu_window":     "Window",
		"menu_minimize":   "Minimize",
		"menu_close":      "Close",

		"err_not_logged_in_title": "Claude Code isn't logged in",
		"err_not_logged_in_body":  "Log in, and reading resumes by itself.",
		"err_not_found_title":     "Can't find claude",
		"err_not_found_body":      "Set the path to claude in Settings.",
		"err_start_title":         "Can't start claude",
		"err_start_body":          "Check the path to claude in Settings, and that the file can be run.",
		"err_timeout_title":       "claude isn't responding",
		"err_timeout_body":        "claude will be restarted for the next reading.",
		"err_exited_title":        "claude quit unexpectedly",
		"err_exited_body":         "claude will be restarted for the next reading.",
		"err_no_limits_title":     "/usage shows no usage",
		"err_no_limits_body":      "claude is logged in. It will be restarted for the next reading.",
		"err_format_title":        "Can't read the /usage output",
		"err_format_body":         "A Claude Code update may have changed its output.",
		"err_other_title":         "Couldn't read usage",
		"err_other_body":          "",
		"login_button":            "Log In…",
		"open_settings_button":    "Open Settings…",
		"login_failed":            "Couldn't open Terminal: {error}",
		"notify_logged_out_title": "Claude Code isn't logged in",
		"notify_logged_out_body":  "Usage can't be read. Log in, and reading resumes by itself.",
		"expiry_days":             "Your login expires in {days} days",
		"expiry_trust":            "To check when your login expires, the working folder has to be trusted once.",
		"trust_button":            "Trust the Folder…",
		"trust_failed":            "Couldn't open Terminal: {error}",
		"trust_script_message":    "claude-usage-visualizer: claude starts in this working folder to check when your login expires. When it asks whether to trust the folder, choose \"Yes, I trust this folder\", then quit claude with /exit. The folder belongs to claude-usage-visualizer and is empty.",
		"login_expiry_check":      "Check when the login expires (starts the interactive claude for a few seconds, about once a day)",
		"compact_session":         "5h",
		"compact_week":            "week",
		"compact_pending":         "Not read yet",
		"compact_failed":          "Can't read usage",
		"menu_compact":            "Compact View",
		"start_compact":           "Start in the compact view",
		"menu_view":               "View",
		"menu_settings_menu":      "Settings",
		"theme_label":             "Look:",
		"theme_platinum":          "Platinum (Mac OS 8 style)",
		"theme_system7":           "System 7 style",
		"settings_title":          "Settings",
		"interval_label":          "Interval:",
		"interval_unit":           "minutes (1–60)",
		"claude_path_label":       "claude path:",
		"claude_path_hint":        "Leave empty to find it automatically",
		"always_on_top":           "Keep the window above other windows",
		"language_label":          "Language:",
		"language_system":         "Same as the OS",
		"close_label":             "When the window is closed:",
		"close_ask":               "Ask whether to quit",
		"close_close":             "Close without asking (keep recording)",
		"close_quit":              "Quit without asking",
		"settings_file":           "Settings file: {path}",
		"logs_dir":                "Records: {path}",
		"cancel":                  "Cancel",
		"save":                    "Save",
		"save_failed":             "Couldn't save: {error}",
		"close_alert_title":       "Quit the app too?",
		"close_alert_message":     "If you only close the window, the app keeps running and recording usage.",
		"close_alert_quit":        "Quit",
		"close_alert_close":       "Just Close",
		"close_alert_cancel":      "Cancel",
	},
}

// LanguageNames are the choices for the language setting, each written in
// its own language, as language menus usually are. The "system" choice is
// in the table above, since it reads in the current language.
var LanguageNames = map[settings.Language]string{
	settings.LanguageJapanese: "日本語",
	settings.LanguageEnglish:  "English",
}
