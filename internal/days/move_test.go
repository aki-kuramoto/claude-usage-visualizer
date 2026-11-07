package days

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 4, 9, 0, 0, 0, tokyo)

const week = "\tweekdays = {\n\t\tmon = 'w'; tue = 'w'; wed = 'w'; thu = 'w'; fri = 'w';\n\t\tsat = 'o'; sun = 'o';\n\t};\n"

func TestMoveOwnLinesWithComments(t *testing.T) {
	src := "# mine\n{\n\tkinds = { w = 1; o = 0; };\n" + week +
		"\tdays = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-10-13', # sports day\n" +
		"\t\t\t'2026-11-03', # culture day\n" +
		"\t\t\t'2026-11-03..2026-11-05', # still going\n" +
		"\t\t\t'2026-12-29..2027-01-03',\n" +
		"\t\t];\n" +
		"\t};\n" +
		"}\n"
	want := "# mine\n{\n\tkinds = { w = 1; o = 0; };\n" + week +
		"\tdays = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-11-03..2026-11-05', # still going\n" +
		"\t\t\t'2026-12-29..2027-01-03',\n" +
		"\t\t];\n" +
		"\t};\n" +
		"\tpast_days = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-10-13', # sports day\n" +
		"\t\t\t'2026-11-03', # culture day\n" +
		"\t\t];\n" +
		"\t};\n" +
		"}\n"
	got, moved, err := MovePast(src, today)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if len(moved) != 2 || moved[0].Comment != "# sports day" || moved[1].Entry != "2026-11-03" {
		t.Errorf("moved %+v", moved)
	}
}

func TestMoveIntoExistingPastLists(t *testing.T) {
	src := "{\n\tkinds = { w = 1; o = 0; };\n" + week +
		"\tdays = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-11-03', # culture day\n" +
		"\t\t];\n" +
		"\t\tw = [ '2026-11-01', '2026-11-08' ];\n" +
		"\t};\n" +
		"\tpast_days = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-09-21',\n" +
		"\t\t];\n" +
		"\t};\n" +
		"}\n"
	want := "{\n\tkinds = { w = 1; o = 0; };\n" + week +
		"\tdays = {\n" +
		"\t\to = [\n" +
		"\t\t];\n" +
		"\t\tw = [ '2026-11-08' ];\n" +
		"\t};\n" +
		"\tpast_days = {\n" +
		"\t\to = [\n" +
		"\t\t\t'2026-09-21',\n" +
		"\t\t\t'2026-11-03', # culture day\n" +
		"\t\t];\n" +
		"\t\tw = [\n" +
		"\t\t\t'2026-11-01',\n" +
		"\t\t];\n" +
		"\t};\n" +
		"}\n"
	got, moved, err := MovePast(src, today)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || len(moved) != 2 {
		t.Errorf("got (%d moved)\n%s\nwant\n%s", len(moved), got, want)
	}

	// Moving again finds nothing to do.
	again, moved, err := MovePast(got, today)
	if err != nil || len(moved) != 0 || again != got {
		t.Errorf("second move: %v %v", moved, err)
	}
}

func TestMoveIntoOneLinePastList(t *testing.T) {
	src := "{\n\tkinds = { o = 0; };\n" +
		"\tweekdays = { mon = 'o'; tue = 'o'; wed = 'o'; thu = 'o'; fri = 'o'; sat = 'o'; sun = 'o'; };\n" +
		"\tdays = { o = [ '2026-11-01' ]; };\n" +
		"\tpast_days = { o = [ '2026-10-01' ]; };\n" +
		"}\n"
	got, moved, err := MovePast(src, today)
	if err != nil || len(moved) != 1 {
		t.Fatalf("%v %v", moved, err)
	}
	c, err := Parse([]byte(got))
	if err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	d, _ := parseDate("2026-11-01")
	if c.KindOf(d) != "o" {
		t.Errorf("the moved date lost its kind:\n%s", got)
	}
}

func TestMoveLeavesBrokenFileAlone(t *testing.T) {
	if _, _, err := MovePast("{ kinds = { w = 1; }; }", today); err == nil {
		t.Error("an invalid file was moved")
	}
}

func TestMoveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "days.setay")
	src := "{\n\tkinds = { w = 1; o = 0; };\n" + week + "\tdays = {\n\t\to = [\n\t\t\t'2026-10-13',\n\t\t];\n\t};\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	moved, err := MoveFile(path, today)
	if err != nil || len(moved) != 1 {
		t.Fatalf("%v %v", moved, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode became %v", st.Mode().Perm())
	}
	if _, err := Parse(mustRead(t, path)); err != nil {
		t.Error(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
