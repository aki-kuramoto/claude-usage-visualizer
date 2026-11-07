package jsonlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSplitsByUTCDate(t *testing.T) {
	dir := t.TempDir()
	d := Daily{Dir: filepath.Join(dir, "records"), Prefix: "usage"}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}

	// 08:30 JST on the 16th is still the 15th in UTC; 09:30 JST is the 16th.
	before := time.Date(2026, 10, 16, 8, 30, 0, 0, tokyo)
	after := time.Date(2026, 10, 16, 9, 30, 0, 0, tokyo)
	for _, w := range []struct {
		at time.Time
		v  any
	}{{before, map[string]int{"n": 1}}, {before, map[string]int{"n": 2}}, {after, map[string]int{"n": 3}}} {
		if err := d.Append(w.at, w.v); err != nil {
			t.Fatal(err)
		}
	}

	check := func(name, want string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dir, "records", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	check("usage-2026-10-15.jsonl", "{\"n\":1}\n{\"n\":2}\n")
	check("usage-2026-10-16.jsonl", "{\"n\":3}\n")
}

func TestRecreatesRemovedFile(t *testing.T) {
	d := Daily{Dir: t.TempDir(), Prefix: "usage"}
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if err := d.Append(at, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(d.Path(at)); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(at, 2); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(d.Path(at))
	if string(got) != "2\n" {
		t.Errorf("got %q", got)
	}
}

func TestUnmarshalableValue(t *testing.T) {
	d := Daily{Dir: t.TempDir(), Prefix: "usage"}
	at := time.Now()
	if err := d.Append(at, func() {}); err == nil {
		t.Error("a function was written")
	}
	if _, err := os.Stat(d.Path(at)); !os.IsNotExist(err) {
		t.Error("a file was created for a line that could not be made")
	}
}
