package uuidv7

import (
	"regexp"
	"sort"
	"testing"
	"time"
)

var shape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestShape(t *testing.T) {
	for range 1000 {
		if id := New(); !shape.MatchString(id) {
			t.Fatalf("%q is not a version 7 UUID", id)
		}
	}
}

func TestTimestampRoundTrip(t *testing.T) {
	at := time.Date(2026, 10, 15, 9, 30, 0, 123_000_000, time.UTC)
	id := NewAt(at)
	if got := id[:13]; got != "01a13ee5-a63b" {
		t.Errorf("prefix = %s, want 01a13ee5-a63b", got)
	}
	got, ok := Time(id)
	if !ok || !got.Equal(at) {
		t.Errorf("Time(%s) = %v, %v; want %v", id, got, ok, at)
	}
}

func TestTimeRejects(t *testing.T) {
	for _, s := range []string{
		"",
		"not-a-uuid",
		"01a13ee5-a83b-4c1e-9d2a-5f8e0b7a4c31", // version 4
		"01a13ee5-a83b-7c1e-cd2a-5f8e0b7a4c31", // variant 110
		"01a13ee5-a83b-7c1e-9d2a-5f8e0b7a4cz1", // not hex
	} {
		if _, ok := Time(s); ok {
			t.Errorf("Time(%q) accepted", s)
		}
	}
}

func TestSortsByTime(t *testing.T) {
	base := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := range 100 {
		ids = append(ids, NewAt(base.Add(time.Duration(i)*time.Millisecond)))
	}
	if !sort.StringsAreSorted(ids) {
		t.Error("IDs made at increasing times do not sort in that order")
	}
}

func TestUnique(t *testing.T) {
	at := time.Now()
	seen := map[string]bool{}
	for range 10000 {
		id := NewAt(at)
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
	}
}
