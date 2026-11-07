package days

import (
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"
)

// Watcher keeps the calendar of one days.setay up to date, reading the file
// again whenever it has changed: people and agents edit it while the
// application runs.
type Watcher struct {
	path     string
	onChange func(c *Calendar, err error, exists bool)

	mu      sync.Mutex
	checked bool
	modTime time.Time
	size    int64
	exists  bool
	cal     *Calendar
	err     error
}

// NewWatcher watches path. onChange, if not nil, is called (with the
// watcher's lock held, so it must not call back) every time the file is read
// again, including the first time.
func NewWatcher(path string, onChange func(c *Calendar, err error, exists bool)) *Watcher {
	return &Watcher{path: path, onChange: onChange}
}

// Path is the file being watched.
func (w *Watcher) Path() string { return w.path }

// Current returns the calendar, or nil when there is no file (the feature is
// simply off) or the file cannot be used (err says why).
func (w *Watcher) Current() (*Calendar, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st, statErr := os.Stat(w.path)
	exists := statErr == nil
	if w.checked && exists == w.exists && (!exists || (st.ModTime().Equal(w.modTime) && st.Size() == w.size)) {
		return w.cal, w.err
	}
	w.checked, w.exists, w.cal, w.err = true, exists, nil, nil
	switch {
	case errors.Is(statErr, fs.ErrNotExist):
	case statErr != nil:
		w.err = statErr
	default:
		w.modTime, w.size = st.ModTime(), st.Size()
		data, err := os.ReadFile(w.path)
		if err == nil {
			w.cal, err = Parse(data)
		}
		w.err = err
	}
	if w.onChange != nil {
		w.onChange(w.cal, w.err, w.exists)
	}
	return w.cal, w.err
}
