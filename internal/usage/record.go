// Package usage turns the text that Claude Code's /usage command prints into
// the records this application keeps.
//
// A record holds the text as it was printed ("raw") and the same values in a
// form that leaves nothing to interpretation ("norm"). Keeping both means a
// mistake in the conversion can be corrected later from the raw text. The
// format is described in README.md.
package usage

import "time"

// FormatVersion is the "v" field of every record.
const FormatVersion = 1

// Record is one line of the usage record.
//
// Raw, Norm and Error are each nil when there is nothing to put in them: a
// reading that failed before any text arrived has only Error, and one whose
// text could be cut up but not converted has Raw and Error.
type Record struct {
	V     int    `json:"v"`
	ID    string `json:"id"`
	M     Method `json:"m"`
	At    string `json:"at"`
	OK    bool   `json:"ok"`
	Raw   *Raw   `json:"raw"`
	Norm  *Norm  `json:"norm"`
	Error *Error `json:"error"`
}

// Raw holds pieces of the /usage text, unchanged.
type Raw struct {
	// Source is the first line, which says what is paying for the usage. It
	// is nil when the text has no such line.
	Source *string    `json:"source"`
	Limits []RawLimit `json:"limits"`
}

// RawLimit is one limit line such as
// "Current session: 35% used · resets Oct 15 at 2:20pm (Asia/Tokyo)".
type RawLimit struct {
	Label string `json:"label"`
	// Usage is the "35% used" part.
	Usage string `json:"usage"`
	// ResetsAt is what follows "resets", including the time zone in
	// brackets. It is nil when the line gives no reset time.
	ResetsAt *string `json:"resets_at"`
}

// Norm holds the values of Raw converted into fixed forms.
type Norm struct {
	// Source is one of the Source constants, or nil when Raw.Source is nil.
	Source *Source     `json:"source"`
	Limits []NormLimit `json:"limits"`
}

// NormLimit is a RawLimit converted.
type NormLimit struct {
	Label   string `json:"label"`
	UsedPct int    `json:"used_pct"`
	// ResetsAt is RFC 3339 in UTC, or nil when the raw line had no reset
	// time.
	ResetsAt *string `json:"resets_at"`
}

// Source says what is paying for the usage.
type Source string

const (
	SourceSubscription Source = "subscription"
	SourceOverage      Source = "overage"
)

// Error describes why a reading did not produce a complete record. The full
// details (standard error, the whole output) belong in the operation log;
// Detail is a short explanation for a person.
type Error struct {
	Stage  Stage  `json:"stage"`
	Reason Reason `json:"reason"`
	Detail string `json:"detail"`
}

// Stage is where a reading failed.
type Stage string

const (
	StageSpawn     Stage = "spawn"     // starting claude
	StageSend      Stage = "send"      // writing the /usage request
	StageWait      Stage = "wait"      // waiting for the result
	StageParse     Stage = "parse"     // cutting the text into Raw
	StageNormalize Stage = "normalize" // converting Raw into Norm
	StageAuth      Stage = "auth"      // claude's login, checked when /usage has no limits
)

// Reason is a short fixed token for what went wrong, for counting failures.
type Reason string

const (
	ReasonTimeout          Reason = "timeout"
	ReasonExited           Reason = "exited"
	ReasonUnexpectedFormat Reason = "unexpected_format"
	ReasonNoLimits         Reason = "no_limits"
	ReasonNoTimeZone       Reason = "no_time_zone"
	ReasonUnknownTimeZone  Reason = "unknown_time_zone"
	ReasonNotFound         Reason = "claude_not_found" // claude could not be found
	ReasonStartFailed      Reason = "start_failed"     // claude could not be started
	ReasonResultError      Reason = "result_error"     // claude answered with an error
	ReasonNotLoggedIn      Reason = "not_logged_in"    // claude is not logged in, so /usage has no plan usage
)

// FormatAt formats the time of a reading for the "at" field: RFC 3339 with
// the local offset.
func FormatAt(t time.Time) string {
	return t.Local().Format(time.RFC3339)
}

// Failed makes the record for a reading that produced no text at all.
func Failed(id string, m Method, at time.Time, e Error) Record {
	return Record{V: FormatVersion, ID: id, M: m, At: FormatAt(at), Error: &e}
}

// FromText makes the record for a reading that produced the /usage text.
func FromText(id string, m Method, at time.Time, text string) Record {
	rec := Record{V: FormatVersion, ID: id, M: m, At: FormatAt(at)}
	raw, err := Parse(text)
	if err != nil {
		rec.Error = err
		return rec
	}
	rec.Raw = raw
	norm, err := Normalize(raw, at)
	if err != nil {
		rec.Error = err
		return rec
	}
	rec.Norm = norm
	rec.OK = true
	return rec
}
