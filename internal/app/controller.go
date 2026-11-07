// Package app is the part of the application that does not depend on the
// GUI: it decides when to read usage, reads it through claude, writes both
// records, and hands the result to whatever displays it.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/claude"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/days"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/expiry"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/i18n"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/jsonlog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/oplog"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/paths"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/settings"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/theme"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/usage"
	"github.com/aki-kuramoto/claude-usage-visualizer/internal/uuidv7"
)

// Version is the application's version, written to the operation log.
const Version = "0.1.0"

// RetryDelay is how long to wait before trying again after a failed reading.
// Only one retry is made; after that the normal interval applies.
const RetryDelay = time.Minute

// minuteTick is how often the date and, while logged out, claude's login
// are checked. Tests shorten it.
var minuteTick = time.Minute

// claudeTimeout is how long one request may take. Tests shorten it.
var claudeTimeout = claude.DefaultTimeout

// Status is what the display should show.
type Status string

const (
	// StatusPending: nothing has been read since the application started.
	StatusPending Status = "pending"
	// StatusOK: the latest reading succeeded.
	StatusOK Status = "ok"
	// StatusFailed: the latest reading failed. Limits still hold the last
	// successful values, which must not be shown as current.
	StatusFailed Status = "failed"
)

// Limit is one bar.
type Limit struct {
	Label   string `json:"label"`
	UsedPct int    `json:"used_pct"`
	// ResetsAt is RFC 3339 in UTC, or empty when there is no reset time.
	ResetsAt string `json:"resets_at"`
	// WindowSeconds is the length of the period that ends at ResetsAt, or
	// zero when it is not known. The display places the time marker with it:
	// the period runs from ResetsAt minus WindowSeconds to ResetsAt.
	WindowSeconds int64 `json:"window_seconds"`
	// Short is a short name for the compact view, in the window's language
	// ("5h", "week", or the model's name for a model's weekly limit).
	Short string `json:"short"`
	// Primary marks the two limits the compact view shows: the five-hour
	// limit and the weekly limit for all models.
	Primary bool `json:"primary"`
	// CalendarTicks and EqualTicks are where the bar's ticks go, as shares
	// of the period from 0 to 1 (see ticks).
	CalendarTicks []float64 `json:"calendar_ticks"`
	EqualTicks    []float64 `json:"equal_ticks"`
}

// ticks places two kinds of ticks on a limit's bar.
//
// Calendar ticks are where the local calendar turns over inside the period:
// midnight for a period of a day or more, the hour (14:00, 15:00, ...) for a
// shorter one. Equal ticks divide the period into equal parts counted from
// its start (the reset time minus the period): seven for a week, five for
// five hours, one per day or per hour. The two differ because the period
// starts at the reset time, not at midnight or on the hour.
func ticks(resetAt string, window time.Duration, loc *time.Location) (calendar, equal []float64) {
	reset, err := time.Parse(time.RFC3339, resetAt)
	if err != nil || window <= 0 {
		return nil, nil
	}
	start := reset.Add(-window)
	frac := func(t time.Time) float64 { return t.Sub(start).Seconds() / window.Seconds() }

	unit := time.Hour
	if window >= 24*time.Hour {
		unit = 24 * time.Hour
	}
	if n := int(window / unit); n > 1 && window%unit == 0 {
		for k := 1; k < n; k++ {
			equal = append(equal, float64(k)/float64(n))
		}
	}

	// The first turn of the calendar after start, then each one after it.
	l := start.In(loc)
	var t time.Time
	if unit == 24*time.Hour {
		t = time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc)
	} else {
		t = time.Date(l.Year(), l.Month(), l.Day(), l.Hour()+1, 0, 0, 0, loc)
	}
	for ; t.Before(reset); t = next(t, unit, loc) {
		if t.After(start) {
			calendar = append(calendar, frac(t))
		}
	}
	return calendar, equal
}

// next is the following midnight or hour on loc's calendar.
func next(t time.Time, unit time.Duration, loc *time.Location) time.Time {
	l := t.In(loc)
	if unit == 24*time.Hour {
		return time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc)
	}
	return t.Add(time.Hour)
}

// shortLabel shortens a limit's label for the compact view.
func shortLabel(label string, strings_ map[string]string) string {
	switch {
	case label == "Current session":
		return strings_["compact_session"]
	case label == "Current week (all models)":
		return strings_["compact_week"]
	case strings.HasPrefix(label, "Current week (") && strings.HasSuffix(label, ")"):
		name := strings.TrimSuffix(strings.TrimPrefix(label, "Current week ("), ")")
		return strings.TrimSuffix(name, " only")
	}
	return label
}

// windowOf tells the length of a limit's period from its label, as /usage
// prints it: "Current session" is the five-hour limit, and every
// "Current week (...)" a weekly one. Other labels give zero.
func windowOf(label string) time.Duration {
	switch {
	case label == "Current session":
		return 5 * time.Hour
	case strings.HasPrefix(label, "Current week"):
		return 7 * 24 * time.Hour
	}
	return 0
}

// State is everything the display needs. It is passed to the display as
// JSON, which keeps the boundary with the GUI code to a single string.
type State struct {
	Status   Status  `json:"status"`
	Fetching bool    `json:"fetching"`
	Limits   []Limit `json:"limits"`
	// LastSuccessAt and LastAttemptAt are RFC 3339 with the local offset,
	// or empty.
	LastSuccessAt string `json:"last_success_at"`
	LastAttemptAt string `json:"last_attempt_at"`
	// Error is a short description of the latest failure; ErrorStage and
	// ErrorReason are its stage and reason (see package usage), which the
	// display uses to say what is wrong and what to do.
	Error       string `json:"error"`
	ErrorStage  string `json:"error_stage"`
	ErrorReason string `json:"error_reason"`
	// ErrorKind groups the reasons by what the user should know or do; the
	// display has a heading and an explanation for each (see ErrorKinds).
	ErrorKind string            `json:"error_kind"`
	Settings  settings.Settings `json:"settings"`
	// SettingsPath is where the settings are saved, for the settings window.
	SettingsPath string `json:"settings_path"`
	LogsDir      string `json:"logs_dir"`
	DaysPath     string `json:"days_path"`
	// LoginExpiry is what the latest look at the interactive screen found
	// about the login's expiry (see package expiry), or nil before the
	// first look or when the check is off.
	LoginExpiry *LoginExpiry `json:"login_expiry"`

	// Lang is the language the window speaks, decided from the setting and
	// the OS's preferred languages; Strings is its text (see package i18n)
	// and DateLocale the locale for dates and times in it.
	Lang          i18n.Lang         `json:"lang"`
	Strings       map[string]string `json:"strings"`
	DateLocale    string            `json:"date_locale"`
	LanguageNames map[string]string `json:"language_names"`

	// Theme is the look the window draws itself in, as the setting chooses
	// (see package theme); ThemeIDs are the choices for the setting, whose
	// names are in Strings as "theme_<id>".
	Theme    theme.Theme `json:"theme"`
	ThemeIDs []theme.ID  `json:"theme_ids"`
}

// Controller runs the readings.
type Controller struct {
	paths    paths.Paths
	log      *oplog.Log
	records  jsonlog.Daily
	publish  func(State)
	triggers chan string
	settings chan settings.Settings

	mu        sync.Mutex
	state     State
	resident  *claude.Resident
	preferred []string // the OS's preferred languages

	days *days.Watcher

	claudePath string // the claude found for the resident process
	loggedOut  bool   // the latest reading found claude not logged in

	expiryTriggers chan string
}

// New prepares a controller. publish is called, from the controller's own
// goroutine, every time the state changes.
func New(p paths.Paths, log *oplog.Log, s settings.Settings, publish func(State)) *Controller {
	watcher := days.NewWatcher(p.Days(), func(cal *days.Calendar, err error, exists bool) {
		switch {
		case err != nil:
			log.Add(oplog.KindDaysError, "", map[string]any{"path": p.Days(), "error": err.Error()})
		case exists:
			log.Add(oplog.KindDaysLoaded, "", map[string]any{"path": p.Days(), "kinds": cal.Kinds()})
		default:
			log.Add(oplog.KindDaysLoaded, "", map[string]any{"path": p.Days(), "exists": false})
		}
	})
	return &Controller{
		days:           watcher,
		paths:          p,
		log:            log,
		records:        jsonlog.Daily{Dir: p.Logs(), Prefix: "usage"},
		publish:        publish,
		triggers:       make(chan string, 8),
		expiryTriggers: make(chan string, 1),
		settings:       make(chan settings.Settings, 1),
		state: State{
			Status: StatusPending, Limits: []Limit{}, Settings: s,
			SettingsPath: p.Settings(), LogsDir: p.Logs(), DaysPath: p.Days(),
		},
	}
}

// SetPreferredLanguages tells the controller the OS's preferred languages,
// most preferred first. The window asks the OS for them, since how to ask
// differs by OS.
func (c *Controller) SetPreferredLanguages(langs []string) {
	c.mu.Lock()
	c.preferred = langs
	c.mu.Unlock()
}

// State returns a copy of the current state.
func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.state
	st.Lang = i18n.Resolve(st.Settings.Language, c.preferred)
	st.Strings = i18n.Strings(st.Lang)
	st.Limits = append([]Limit(nil), st.Limits...)
	for i := range st.Limits {
		st.Limits[i].Short = shortLabel(st.Limits[i].Label, st.Strings)
		st.Limits[i].Primary = st.Limits[i].Label == "Current session" || st.Limits[i].Label == "Current week (all models)"
		st.Limits[i].CalendarTicks, st.Limits[i].EqualTicks = ticks(
			st.Limits[i].ResetsAt, time.Duration(st.Limits[i].WindowSeconds)*time.Second, time.Local)
	}
	st.DateLocale = i18n.DateLocale(st.Lang)
	st.LanguageNames = map[string]string{}
	for k, v := range i18n.LanguageNames {
		st.LanguageNames[string(k)] = v
	}
	var ok bool
	if st.Theme, ok = theme.Get(st.Settings.Theme); !ok {
		st.Theme, _ = theme.Get(theme.Default)
	}
	st.ThemeIDs = theme.IDs()
	return st
}

// FetchNow asks for a reading outside the schedule. reason is logged.
func (c *Controller) FetchNow(reason string) {
	select {
	case c.triggers <- reason:
	default: // one is already waiting
	}
}

// SaveSettings validates, saves and applies new settings.
func (c *Controller) SaveSettings(s settings.Settings) error {
	res, err := settings.Save(c.paths.Settings(), s)
	if err != nil {
		c.log.Add(oplog.KindError, "", map[string]any{"what": "saving settings", "error": err.Error()})
		return err
	}
	c.log.Add(oplog.KindSettingsSaved, "", map[string]any{
		"settings": s, "changed": res.Changed, "created": res.Created, "backup": res.Backup,
	})
	select {
	case <-c.settings: // replace one not yet applied
	default:
	}
	c.settings <- s
	return nil
}

// NoteWindow logs that the window was shown or closed.
func (c *Controller) NoteWindow(what string) {
	c.log.Add(oplog.KindWindow, "", map[string]any{"what": what})
}

// NotePower logs that the machine went to sleep or woke, and reads usage
// right after waking.
func (c *Controller) NotePower(what string) {
	c.log.Add(oplog.KindPower, "", map[string]any{"what": what})
	if what == "wake" {
		c.FetchNow("wake")
	}
}

// ClaudePath is the claude the application runs: the one found for the
// resident process, else the configured one, else the one Find picks.
func (c *Controller) ClaudePath() (string, error) {
	c.mu.Lock()
	path, configured := c.claudePath, c.state.Settings.ClaudePath
	c.mu.Unlock()
	if path != "" {
		return path, nil
	}
	path, _, err := claude.Find(configured)
	return path, err
}

// checkAuth asks claude whether it is logged in, and logs the answer.
func (c *Controller) checkAuth(id string) (claude.Auth, error) {
	path, err := c.ClaudePath()
	if err != nil {
		return claude.Auth{}, err
	}
	if err := os.MkdirAll(c.paths.Work(), 0o755); err != nil {
		return claude.Auth{}, err
	}
	a, err := claude.AuthStatus(path, c.paths.Work(), claude.Env(os.Environ(), path))
	if err != nil {
		c.log.Add(oplog.KindError, id, map[string]any{"what": "checking claude's login", "error": err.Error()})
		return a, err
	}
	c.log.Add(oplog.KindAuth, id, map[string]any{"logged_in": a.LoggedIn, "auth_method": a.AuthMethod, "subscription_type": a.SubscriptionType})
	return a, nil
}

// loginRestored is called once a minute. While the latest reading found
// claude not logged in, it checks the login again, and reports when it is
// back so that a reading can be made at once.
func (c *Controller) loginRestored() bool {
	c.mu.Lock()
	out := c.loggedOut
	c.mu.Unlock()
	if !out {
		return false
	}
	a, err := c.checkAuth("")
	if err != nil || !a.LoggedIn {
		return false
	}
	// A fresh claude picks up the new login.
	c.stopResident()
	c.mu.Lock()
	c.loggedOut = false
	c.mu.Unlock()
	return true
}

// NoteLoginOpened logs that a terminal was opened to log in.
func (c *Controller) NoteLoginOpened(script string, err error) {
	data := map[string]any{"script": script}
	if err != nil {
		data["error"] = err.Error()
	}
	c.log.Add(oplog.KindLoginOpened, "", data)
}

// DaysElapsed tells how much of the period that ends at reset has passed at
// now, counting days by their weight in days.setay. ok is false when there is
// nothing to show: no file, a file that cannot be used, a period shorter than
// a day (days do not matter inside the five-hour limit), or a period that
// weighs nothing.
func (c *Controller) DaysElapsed(reset time.Time, window time.Duration, now time.Time) (frac float64, ok bool) {
	if window < 24*time.Hour {
		return 0, false
	}
	cal, err := c.days.Current()
	if cal == nil || err != nil {
		return 0, false
	}
	return cal.Elapsed(reset.Add(-window), reset, now, time.Local)
}

// DaysAhead is like DaysElapsed, but judged by the days still ahead (see
// days.Calendar.ElapsedAhead): it matches the plain share of time when every
// day ahead is of the heaviest kind.
func (c *Controller) DaysAhead(reset time.Time, window time.Duration, now time.Time) (frac float64, ok bool) {
	if window < 24*time.Hour {
		return 0, false
	}
	cal, err := c.days.Current()
	if cal == nil || err != nil {
		return 0, false
	}
	return cal.ElapsedAhead(reset.Add(-window), reset, now, time.Local)
}

// DaysError says why days.setay cannot be used, or is empty.
func (c *Controller) DaysError() string {
	if _, err := c.days.Current(); err != nil {
		return err.Error()
	}
	return ""
}

// createDefaultDays writes a days.setay to start from when there is none,
// so that the usage-days marker can be tried without writing anything.
func (c *Controller) createDefaultDays() {
	created, err := days.CreateDefault(c.paths.Days())
	switch {
	case err != nil:
		c.log.Add(oplog.KindDaysError, "", map[string]any{"path": c.paths.Days(), "while": "writing the default", "error": err.Error()})
	case created:
		c.log.Add(oplog.KindDaysCreated, "", map[string]any{"path": c.paths.Days()})
	}
}

// movePastDays moves the exceptions in days.setay that are over to
// past_days. A missing file is fine; a broken one is left alone (the watcher
// reports it).
func (c *Controller) movePastDays(now time.Time) {
	if _, err := os.Stat(c.paths.Days()); err != nil {
		return
	}
	moved, err := days.MoveFile(c.paths.Days(), now)
	if err != nil {
		c.log.Add(oplog.KindDaysError, "", map[string]any{"path": c.paths.Days(), "while": "moving past days", "error": err.Error()})
		return
	}
	if len(moved) > 0 {
		c.log.Add(oplog.KindDaysMoved, "", map[string]any{"path": c.paths.Days(), "moved": moved})
	}
}

// Run reads usage at start, then on the schedule and on request, until ctx
// ends.
func (c *Controller) Run(ctx context.Context) {
	exe, _ := os.Executable()
	c.log.Add(oplog.KindAppStart, "", map[string]any{
		"version": Version, "os": runtime.GOOS, "arch": runtime.GOARCH, "pid": os.Getpid(),
		"executable": exe, "base": c.paths.Base,
	})
	c.publishState()
	defer func() {
		c.stopResident()
		c.log.Add(oplog.KindAppStop, "", nil)
	}()

	// Past exceptions in days.setay are moved at start and whenever the
	// date changes; a ticker checks the date once a minute.
	today := time.Now().Format(time.DateOnly)
	c.createDefaultDays()
	c.movePastDays(time.Now())
	c.days.Current() // log what the file holds
	dayTicker := time.NewTicker(minuteTick)
	defer dayTicker.Stop()

	// The expiry check runs beside the readings; it is waited for on the way
	// out, so that no interactive claude outlives the application.
	expiryDone := make(chan struct{})
	go func() {
		c.runExpiry(ctx)
		close(expiryDone)
	}()
	defer func() { <-expiryDone }()

	retried := false
	ok := c.fetch("start")
	next := time.Now().Add(c.nextDelay(ok, retried))
	for {
		timer := time.NewTimer(time.Until(next))
		reason := ""
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case now := <-dayTicker.C:
			timer.Stop()
			if d := now.Format(time.DateOnly); d != today {
				today = d
				c.movePastDays(now)
			}
			if !c.loginRestored() {
				continue // the next reading stays when it was
			}
			reason = "login_restored"
			c.CheckExpiryNow("login_restored")
		case <-timer.C:
			reason = "schedule"
			if !ok && !retried {
				reason = "retry"
			}
		case reason = <-c.triggers:
			timer.Stop()
		case s := <-c.settings:
			timer.Stop()
			c.applySettings(s)
			next = time.Now().Add(c.nextDelay(ok, retried)) // the new interval counts from now
			continue
		}
		wasRetry := reason == "retry"
		ok = c.fetch(reason)
		retried = !ok && wasRetry
		next = time.Now().Add(c.nextDelay(ok, retried))
	}
}

// nextDelay is how long to wait after a reading: the interval, or the retry
// delay after a first failure.
func (c *Controller) nextDelay(ok, retried bool) time.Duration {
	if !ok && !retried {
		return RetryDelay
	}
	return c.State().Settings.Interval()
}

func (c *Controller) applySettings(s settings.Settings) {
	c.mu.Lock()
	expiryToggled := c.state.Settings.LoginExpiryCheck != s.LoginExpiryCheck
	pathChanged := c.state.Settings.ClaudePath != s.ClaudePath
	c.state.Settings = s
	c.mu.Unlock()
	if pathChanged {
		c.stopResident()
	}
	if expiryToggled {
		c.CheckExpiryNow("settings") // a look when turned on; clears it when turned off
	}
	c.publishState()
}

// fetch makes one reading and reports whether it succeeded.
func (c *Controller) fetch(reason string) bool {
	at := time.Now()
	id := uuidv7.NewAt(at)
	c.log.AddAt(at, oplog.KindFetchStart, id, map[string]any{"reason": reason})
	c.mu.Lock()
	c.state.Fetching = true
	c.mu.Unlock()
	c.publishState()

	var rec usage.Record
	var ans *claude.Answer
	res, uerr := c.ensureResident(id)
	if uerr == nil {
		ans, uerr = res.Ask(id)
	}
	if uerr != nil {
		rec = usage.Failed(id, usage.MethodStreamJSONResident, at, *uerr)
	} else {
		rec = usage.FromText(id, usage.MethodStreamJSONResident, at, ans.Text)
	}
	loggedOut := false
	if rec.Error != nil && rec.Error.Reason == usage.ReasonNoLimits {
		// /usage shows no plan usage when claude is not logged in. Ask
		// claude's login directly, so that this can be said plainly.
		if a, err := c.checkAuth(id); err == nil && !a.LoggedIn {
			loggedOut = true
			rec.Error = &usage.Error{Stage: usage.StageAuth, Reason: usage.ReasonNotLoggedIn,
				Detail: fmt.Sprintf("claude is not logged in (auth method %q)", a.AuthMethod)}
		} else if err == nil {
			// Logged in, yet no plan usage: the resident claude may hold
			// an old login. Start a fresh one for the next reading.
			c.log.Add(oplog.KindClaudeKill, id, map[string]any{"reason": "logged in but /usage has no limits; restarting claude"})
			c.stopResident()
		}
	}
	if err := c.records.Append(at, rec); err != nil {
		c.log.Add(oplog.KindError, id, map[string]any{"what": "writing the usage record", "error": err.Error()})
	}

	end := map[string]any{
		"ok": rec.OK, "duration_ms": time.Since(at).Milliseconds(), "error": rec.Error,
	}
	if ans != nil {
		end["local_command"] = ans.LocalCommand
		end["result"] = ans.Text
		if len(ans.UsageReport) > 0 {
			end["usage_report"] = json.RawMessage(ans.UsageReport)
		}
		if ans.CPU >= 0 {
			end["cpu_ms"] = ans.CPU.Milliseconds()
		}
	}
	c.log.Add(oplog.KindFetchEnd, id, end)

	c.mu.Lock()
	c.state.Fetching = false
	c.state.LastAttemptAt = rec.At
	c.loggedOut = loggedOut
	if rec.OK {
		c.state.Status = StatusOK
		c.state.LastSuccessAt = rec.At
		c.state.Error, c.state.ErrorStage, c.state.ErrorReason, c.state.ErrorKind = "", "", "", ""
		c.state.Limits = limitsOf(rec.Norm)
	} else {
		c.state.Status = StatusFailed
		c.state.Error = describe(rec.Error)
		c.state.ErrorStage, c.state.ErrorReason = string(rec.Error.Stage), string(rec.Error.Reason)
		c.state.ErrorKind = kindOf(rec.Error.Reason)
	}
	c.mu.Unlock()
	c.publishState()
	return rec.OK
}

func (c *Controller) ensureResident(id string) (*claude.Resident, *usage.Error) {
	c.mu.Lock()
	res := c.resident
	configured := c.state.Settings.ClaudePath
	c.mu.Unlock()
	if res != nil {
		return res, nil
	}

	path, tried, err := claude.Find(configured)
	if err != nil {
		c.log.Add(oplog.KindClaudeFound, id, map[string]any{"found": false, "tried": tried, "error": err.Error()})
		return nil, &usage.Error{Stage: usage.StageSpawn, Reason: usage.ReasonNotFound, Detail: err.Error()}
	}
	c.log.Add(oplog.KindClaudeFound, id, map[string]any{"found": true, "path": path, "tried": tried})
	if err := os.MkdirAll(c.paths.Work(), 0o755); err != nil {
		return nil, &usage.Error{Stage: usage.StageSpawn, Reason: usage.ReasonStartFailed, Detail: err.Error()}
	}
	res = claude.New(claude.Config{
		Path:    path,
		Dir:     c.paths.Work(),
		Env:     claude.Env(os.Environ(), path),
		Timeout: claudeTimeout,
		Log:     c.log,
	})
	c.mu.Lock()
	c.resident = res
	c.claudePath = path
	c.mu.Unlock()
	return res, nil
}

func (c *Controller) stopResident() {
	c.mu.Lock()
	res := c.resident
	c.resident = nil
	c.mu.Unlock()
	if res != nil {
		res.Stop()
	}
}

func (c *Controller) publishState() {
	if c.publish != nil {
		c.publish(c.State())
	}
}

func limitsOf(n *usage.Norm) []Limit {
	out := []Limit{}
	if n == nil {
		return out
	}
	for _, l := range n.Limits {
		v := Limit{Label: l.Label, UsedPct: l.UsedPct, WindowSeconds: int64(windowOf(l.Label) / time.Second)}
		if l.ResetsAt != nil {
			v.ResetsAt = *l.ResetsAt
		}
		out = append(out, v)
	}
	return out
}

// LoginExpiry is the result of reading the interactive screen.
type LoginExpiry struct {
	Status    expiry.Status `json:"status"`
	DaysLeft  int           `json:"days_left,omitempty"`
	CheckedAt string        `json:"checked_at"`
}

// ExpiryInterval is how often the login's expiry is checked, and
// TrustRecheck how often while the working directory still needs trusting
// (a look then costs a fraction of a second: the dialog shows at once).
var (
	ExpiryInterval = 24 * time.Hour
	TrustRecheck   = 10 * time.Minute
	expiryFirst    = 15 * time.Second // after start, once the first reading is done
)

// CheckExpiryNow asks for a look at the login's expiry outside the schedule.
func (c *Controller) CheckExpiryNow(reason string) {
	select {
	case c.expiryTriggers <- reason:
	default:
	}
}

// runExpiry checks the login's expiry on its own schedule until ctx ends.
func (c *Controller) runExpiry(ctx context.Context) {
	next := time.Now().Add(expiryFirst)
	for {
		timer := time.NewTimer(time.Until(next))
		reason := "schedule"
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case reason = <-c.expiryTriggers:
			timer.Stop()
		}
		status := c.checkExpiry(ctx, reason)
		wait := ExpiryInterval
		if status == expiry.StatusTrustNeeded {
			wait = TrustRecheck
		}
		next = time.Now().Add(wait)
	}
}

// checkExpiry looks at the interactive screen once, unless the check is off
// or claude is not logged in (then there is no login to expire, and the
// screen would say so only after some ten seconds).
func (c *Controller) checkExpiry(ctx context.Context, reason string) expiry.Status {
	if !c.State().Settings.LoginExpiryCheck {
		c.setExpiry(nil)
		return ""
	}
	start := time.Now()
	var r expiry.Result
	a, err := c.checkAuth("")
	switch {
	case err != nil:
		r = expiry.Result{Status: expiry.StatusUnknown, Excerpt: err.Error()}
	case !a.LoggedIn:
		r = expiry.Result{Status: expiry.StatusLoggedOut}
	default:
		path, perr := c.ClaudePath()
		if perr != nil {
			r = expiry.Result{Status: expiry.StatusUnknown, Excerpt: perr.Error()}
			break
		}
		if err := os.MkdirAll(c.paths.Work(), 0o755); err != nil {
			r = expiry.Result{Status: expiry.StatusUnknown, Excerpt: err.Error()}
			break
		}
		r, err = expiry.Probe(ctx, path, c.paths.Work(), claude.Env(os.Environ(), path))
		if err != nil {
			r = expiry.Result{Status: expiry.StatusUnknown, Excerpt: err.Error()}
		}
	}
	c.log.Add(oplog.KindLoginExpiry, "", map[string]any{
		"reason": reason, "result": r, "duration_ms": time.Since(start).Milliseconds(),
	})
	c.setExpiry(&LoginExpiry{Status: r.Status, DaysLeft: r.DaysLeft, CheckedAt: FormatTime(time.Now())})
	return r.Status
}

func (c *Controller) setExpiry(e *LoginExpiry) {
	c.mu.Lock()
	c.state.LoginExpiry = e
	c.mu.Unlock()
	c.publishState()
}

// FormatTime is how the state writes times: RFC 3339 with the local offset.
func FormatTime(t time.Time) string { return usage.FormatAt(t) }

// NoteTrustOpened logs that a terminal was opened to trust the working
// directory, and looks again a little later, when the user may be done.
func (c *Controller) NoteTrustOpened(script string, err error) {
	data := map[string]any{"script": script}
	if err != nil {
		data["error"] = err.Error()
	}
	c.log.Add(oplog.KindTrustOpened, "", data)
}

// ErrorKinds are the values of State.ErrorKind. The display has, for each, a
// heading "err_<kind>_title" and an explanation "err_<kind>_body" in package
// i18n.
var ErrorKinds = []string{"not_logged_in", "not_found", "start", "timeout", "exited", "no_limits", "format", "other"}

// kindOf groups a failure reason by what the user should know or do.
func kindOf(r usage.Reason) string {
	switch r {
	case usage.ReasonNotLoggedIn:
		return "not_logged_in" // log in; reading resumes by itself
	case usage.ReasonNotFound:
		return "not_found" // set the path to claude
	case usage.ReasonStartFailed:
		return "start"
	case usage.ReasonTimeout:
		return "timeout" // claude is restarted for the next reading
	case usage.ReasonExited, usage.ReasonResultError:
		return "exited"
	case usage.ReasonNoLimits:
		return "no_limits" // logged in, yet no plan usage
	case usage.ReasonUnexpectedFormat, usage.ReasonNoTimeZone, usage.ReasonUnknownTimeZone:
		return "format" // Claude Code's output may have changed
	}
	return "other"
}

// WorkDir is where claude runs, and where the login script is written.
func (c *Controller) WorkDir() string { return c.paths.Work() }

func describe(e *usage.Error) string {
	if e == nil {
		return ""
	}
	return string(e.Stage) + " / " + string(e.Reason) + ": " + e.Detail
}
