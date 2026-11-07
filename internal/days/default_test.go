package days

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultText(t *testing.T) {
	c, err := Parse([]byte(DefaultText))
	if err != nil {
		t.Fatal(err)
	}
	// Thursday 07:00 to Thursday 07:00: weekdays weigh 9, the weekend 1.
	start, end := at(2026, 10, 15, 7), at(2026, 10, 22, 7)
	total := 9*(17.0/24) + 9 + 1 + 1 + 9*3 + 9*(7.0/24)
	got, ok := c.Elapsed(start, end, at(2026, 10, 17, 0), tokyo)
	if want := (9*(17.0/24) + 9) / total; !ok || !near(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if out, moved, err := MovePast(DefaultText, time.Now()); err != nil || len(moved) != 0 || out != DefaultText {
		t.Errorf("moving past days changed the default: %v %v", moved, err)
	}
}

func TestCreateDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "days.setay")
	created, err := CreateDefault(path)
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	os.WriteFile(path, []byte("mine"), 0o644)
	created, err = CreateDefault(path)
	if err != nil || created || string(mustRead(t, path)) != "mine" {
		t.Errorf("an existing file was touched: %v %v", created, err)
	}
}
