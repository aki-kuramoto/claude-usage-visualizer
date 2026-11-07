package days

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	setay "github.com/aki-kuramoto/setay-go"
)

// Moved is one entry that was moved from days to past_days.
type Moved struct {
	Kind    string `json:"kind"`
	Entry   string `json:"entry"`
	Comment string `json:"comment,omitempty"`
}

// MovePast moves the entries of days whose last date is before today into
// past_days, under the same kind, and returns the new text. Everything else
// in the file stays as written.
//
// An entry that sits on a line of its own is removed with its whole line,
// and a comment at the end of that line goes with it to past_days. An entry
// that shares a line with others is removed by rewriting that list on one
// line. setay-go's Delete is not used for either, since it leaves such a
// comment behind on a line of its own.
//
// The result is checked before it is returned: it must read as the same
// calendar, with exactly the moved entries gone from days and added to
// past_days.
func MovePast(src string, today time.Time) (string, []Moved, error) {
	var before File
	if err := setay.Unmarshal([]byte(src), &before, setay.DisallowUnknownFields()); err != nil {
		return "", nil, err
	}
	if _, err := before.check(); err != nil {
		return "", nil, err
	}
	doc, err := setay.ParseDocument(src)
	if err != nil {
		return "", nil, err
	}
	todayDate := dateOf(today)
	text := []rune(src)
	nl := "\n"
	if strings.Contains(src, "\r\n") {
		nl = "\r\n"
	}

	var splices []splice
	var moved []Moved
	for _, kind := range sortedKeys(before.Days) {
		list, ok := doc.Get(":/days." + kind)
		if !ok || list.Kind() != "list" {
			continue
		}
		var keep []string // the raw text of the entries that stay
		var lineCuts []splice
		allOwnLines := true
		var movedHere []Moved
		for i := 0; ; i++ {
			elem, ok := list.Index(i)
			if !ok {
				break
			}
			var s string
			if err := elem.Unmarshal(&s); err != nil {
				return "", nil, err
			}
			sp, err := ParseSpan(s)
			if err != nil {
				return "", nil, err
			}
			if !sp.Last.before(todayDate) {
				keep = append(keep, elem.Raw())
				continue
			}
			start, length := elem.Span()
			line, comment, own := ownLine(text, start, start+length)
			if !own {
				allOwnLines = false
			}
			lineCuts = append(lineCuts, line)
			movedHere = append(movedHere, Moved{Kind: kind, Entry: s, Comment: comment})
		}
		if len(movedHere) == 0 {
			continue
		}
		if allOwnLines {
			splices = append(splices, lineCuts...)
		} else {
			// Rewrite the whole list on one line. Comments inside such a
			// list are lost, but a list written on one line seldom has any.
			start, length := list.Span()
			with := "[]"
			if len(keep) > 0 {
				with = "[ " + strings.Join(keep, ", ") + " ]"
			}
			splices = append(splices, splice{start, start + length, with})
			for i := range movedHere {
				movedHere[i].Comment = ""
			}
		}
		moved = append(moved, movedHere...)
	}
	if len(moved) == 0 {
		return src, nil, nil
	}

	// Phase one: take the entries out, from the end so that earlier offsets
	// stay valid.
	sort.Slice(splices, func(i, j int) bool { return splices[i].start > splices[j].start })
	for _, s := range splices {
		text = append(text[:s.start], append([]rune(s.with), text[s.end:]...)...)
	}
	removed := string(text)

	// Phase two: add them to past_days.
	doc, err = setay.ParseDocument(removed)
	if err != nil {
		return "", nil, fmt.Errorf("after taking the entries out: %w", err)
	}
	cs := doc.Changes()
	byKind := map[string][]Moved{}
	var kinds []string
	for _, m := range moved {
		if _, ok := byKind[m.Kind]; !ok {
			kinds = append(kinds, m.Kind)
		}
		byKind[m.Kind] = append(byKind[m.Kind], m)
	}
	past, hasPast := doc.Field("past_days")
	if !hasPast {
		var b strings.Builder
		b.WriteString("{" + nl)
		for _, k := range kinds {
			b.WriteString("\t\t" + k + " = " + multiLineList(nil, byKind[k], "\t\t", nl) + ";" + nl)
		}
		b.WriteString("\t}")
		if err := appendEntry(cs, doc.Root(), "past_days", b.String(), "\t", nl); err != nil {
			return "", nil, err
		}
	} else {
		for _, k := range kinds {
			list, ok := past.Field(k)
			if !ok {
				if err := appendEntry(cs, past, k, multiLineList(nil, byKind[k], "\t\t", nl), "\t\t", nl); err != nil {
					return "", nil, err
				}
				continue
			}
			if err := appendToList(cs, list, byKind[k], nl); err != nil {
				return "", nil, err
			}
		}
	}
	result, err := cs.Apply()
	if err != nil {
		return "", nil, fmt.Errorf("after adding the entries to past_days: %w", err)
	}
	out := result.String()
	if err := checkMove(before, out, moved); err != nil {
		return "", nil, err
	}
	return out, moved, nil
}

// splice replaces text[start:end] with with.
type splice struct {
	start, end int
	with       string
}

// ownLine reports whether the entry at text[start:end] is alone on its line
// (apart from a separating comma and a comment), and if so the range of the
// whole line and the comment.
func ownLine(text []rune, start, end int) (line splice, comment string, own bool) {
	lineStart := start
	for lineStart > 0 && text[lineStart-1] != '\n' {
		lineStart--
	}
	for _, r := range text[lineStart:start] {
		if r != ' ' && r != '\t' {
			return line, "", false
		}
	}
	i := end
	skip := func() {
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	}
	skip()
	if i < len(text) && text[i] == ',' {
		i++
	}
	skip()
	if i < len(text) && text[i] == '#' {
		j := i
		for j < len(text) && text[j] != '\n' && text[j] != '\r' {
			j++
		}
		comment = strings.TrimSpace(string(text[i:j]))
		i = j
	}
	if i < len(text) && text[i] == '\r' {
		i++
	}
	if i < len(text) && text[i] != '\n' {
		return line, "", false
	}
	if i < len(text) {
		i++ // the newline itself
	}
	line.start, line.end = lineStart, i
	return line, comment, true
}

// line is one entry as it is written into past_days.
func (m Moved) line() string {
	s := quote(m.Entry) + ","
	if m.Comment != "" {
		s += " " + m.Comment
	}
	return s
}

func quote(s string) string {
	// Dates and ranges hold only digits, '-' and '.', so plain quotes do.
	return "'" + s + "'"
}

// multiLineList writes a list with one entry on each line. indent is the
// indentation of the line the list starts on.
func multiLineList(existing []string, moved []Moved, indent, nl string) string {
	var b strings.Builder
	b.WriteString("[" + nl)
	for _, e := range existing {
		b.WriteString(indent + "\t" + e + "," + nl)
	}
	for _, m := range moved {
		b.WriteString(indent + "\t" + m.line() + nl)
	}
	b.WriteString(indent + "]")
	return b.String()
}

// appendToList adds entries to an existing list in past_days. A list whose
// closing bracket is on a line of its own gets each entry as a new line just
// above it; any other list is rewritten with one entry on each line.
func appendToList(cs *setay.ChangeSet, list *setay.Node, moved []Moved, nl string) error {
	raw := list.Raw()
	if last := strings.LastIndex(raw, "\n"); last >= 0 && strings.TrimSpace(raw[last+1:len(raw)-1]) == "" && strings.HasSuffix(raw, "]") {
		closingIndent := strings.TrimRight(raw[last+1:len(raw)-1], "\r")
		var b strings.Builder
		for _, m := range moved {
			b.WriteString("\t" + m.line() + nl + closingIndent)
		}
		return cs.AppendRaw(list, b.String())
	}
	var existing []string
	for i := 0; ; i++ {
		elem, ok := list.Index(i)
		if !ok {
			break
		}
		existing = append(existing, elem.Raw())
	}
	return cs.SetRaw(list, multiLineList(existing, moved, "\t\t", nl))
}

// appendEntry adds key = text at the end of dict. setay-go's AppendEntry
// refuses an empty dict, which then gets the entry through AppendRaw with the
// given indentation.
func appendEntry(cs *setay.ChangeSet, dict *setay.Node, key, text, indent, nl string) error {
	err := cs.AppendEntry(dict, key, text)
	if err == nil || !strings.Contains(err.Error(), "empty dict") {
		return err
	}
	return cs.AppendRaw(dict, nl+indent+key+" = "+text+";"+nl)
}

// checkMove makes sure that out is before with the moved entries taken from
// days and added to past_days, and nothing else changed.
func checkMove(before File, out string, moved []Moved) error {
	var after File
	if err := setay.Unmarshal([]byte(out), &after, setay.DisallowUnknownFields()); err != nil {
		return fmt.Errorf("the moved file does not read back: %w", err)
	}
	want := before
	want.Days = map[string][]string{}
	want.PastDays = map[string][]string{}
	for k, v := range before.Days {
		want.Days[k] = append([]string{}, v...)
	}
	for k, v := range before.PastDays {
		want.PastDays[k] = append([]string{}, v...)
	}
	for _, m := range moved {
		list := want.Days[m.Kind]
		for i, e := range list {
			if e == m.Entry {
				want.Days[m.Kind] = append(list[:i], list[i+1:]...)
				break
			}
		}
		want.PastDays[m.Kind] = append(want.PastDays[m.Kind], m.Entry)
	}
	if !sameFile(want, after) {
		return errors.New("the moved file does not read as expected")
	}
	return nil
}

func sameFile(a, b File) bool {
	norm := func(m map[string][]string) map[string][]string {
		out := map[string][]string{}
		for k, v := range m {
			if len(v) > 0 {
				out[k] = v
			}
		}
		return out
	}
	return reflect.DeepEqual(a.Kinds, b.Kinds) && reflect.DeepEqual(a.Weekdays, b.Weekdays) &&
		reflect.DeepEqual(norm(a.Days), norm(b.Days)) && reflect.DeepEqual(norm(a.PastDays), norm(b.PastDays))
}

func sortedKeys(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MoveFile applies MovePast to the file at path. The file is read again just
// before writing, and left alone if it changed in the meantime (someone was
// editing it); the next call tries again. It returns what was moved.
func MoveFile(path string, today time.Time) ([]Moved, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out, moved, err := MovePast(string(data), today)
	if err != nil || len(moved) == 0 {
		return nil, err
	}
	again, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(again, data) {
		return nil, errors.New("the file changed while past days were being moved; trying again later")
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), st.Mode().Perm()); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return moved, nil
}
