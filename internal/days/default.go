package days

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultText is the days.setay written when there is none, so that someone
// trying the application sees the usage-days marker without writing anything
// first: weekdays weigh 9 and weekends 1, with comments on how to change it.
const DefaultText = `# The days of claude-usage-visualizer.
# The triangles at the weekly bars (the time elapsed counted by the days you
# use Claude) are worked out from this file.
# The application wrote it because there was none. Edit it freely: the
# application picks up the changes. How to write it: docs/days-setay.md of
# claude-usage-visualizer.
{
	# The kinds of days, each with a weight for how much you use Claude on it.
	# Name them as you like. Only the ratios matter (0 for days off).
	kinds = {
		weekday = 9;
		weekend = 1;
	};

	# The kind of each day of the week. Write all seven.
	weekdays = {
		mon = 'weekday'; tue = 'weekday'; wed = 'weekday'; thu = 'weekday'; fri = 'weekday';
		sat = 'weekend'; sun = 'weekend';
	};

	# Exceptions: a list of dates for each kind. A range is 'from..to', both
	# ends included. The application moves dates that have passed to past_days.
	days = {
		# weekend = [
		# 	'2026-11-03', # a public holiday
		# 	'2026-12-29..2027-01-03', # New Year holidays
		# ];
	};
}
`

// CreateDefault writes DefaultText to path when there is no file there, and
// reports whether it did. An existing file is never touched.
func CreateDefault(path string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(DefaultText); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}
