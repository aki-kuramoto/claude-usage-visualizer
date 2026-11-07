package oplog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWritesOneLinePerEvent(t *testing.T) {
	dir := t.TempDir()
	var echo bytes.Buffer
	l := New(dir, &echo, nil)
	at := time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	l.AddAt(at, KindAppStart, "", map[string]any{"version": "x"})
	l.AddAt(at, KindFetchStart, "01a1", nil)

	f, err := os.Open(filepath.Join(dir, "ops-2026-10-16.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if len(events) != 2 || events[0].Kind != KindAppStart || events[1].ID != "01a1" || events[1].Data != nil {
		t.Errorf("events = %+v", events)
	}
	if bytes.Count(echo.Bytes(), []byte("\n")) != 2 {
		t.Errorf("echo = %q", echo.String())
	}
}
