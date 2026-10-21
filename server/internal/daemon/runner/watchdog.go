package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// InactiveError reports an agent that went silent past the inactivity timeout.
type InactiveError struct {
	After  time.Duration
	Detail string // last agent-side error, e.g. "Rate limit exceeded"
}

func (e *InactiveError) Error() string {
	msg := fmt.Sprintf("agent produced no output for %s", e.After)
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

// watchdog fires when no activity is seen for d, except while held (a human
// decision is pending) or while the paused func reports true.
type watchdog struct {
	d      time.Duration
	paused func() bool
	mu     sync.Mutex
	last   time.Time
	holds  int
	fired  atomic.Bool
}

func newWatchdog(d time.Duration, paused func() bool) *watchdog {
	return &watchdog{d: d, paused: paused, last: time.Now()}
}

func (w *watchdog) touch() {
	w.mu.Lock()
	w.last = time.Now()
	w.mu.Unlock()
}

func (w *watchdog) hold() { w.mu.Lock(); w.holds++; w.mu.Unlock() }

func (w *watchdog) release() {
	w.mu.Lock()
	w.holds--
	w.last = time.Now()
	w.mu.Unlock()
}

// start runs the watchdog until fire or the returned func is called; the
// returned func reports whether it fired.
func (w *watchdog) start(fire context.CancelFunc) func() bool {
	if w.d <= 0 {
		return func() bool { return false }
	}
	stop := make(chan struct{})
	var once sync.Once
	tick := w.d / 20
	if tick < 10*time.Millisecond {
		tick = 10 * time.Millisecond
	}
	if tick > 5*time.Second {
		tick = 5 * time.Second
	}
	go func() {
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				w.mu.Lock()
				if w.holds > 0 || (w.paused != nil && w.paused()) {
					w.last = time.Now()
				}
				idle := time.Since(w.last)
				w.mu.Unlock()
				if idle >= w.d {
					w.fired.Store(true)
					fire()
					return
				}
			}
		}
	}()
	return func() bool {
		once.Do(func() { close(stop) })
		return w.fired.Load()
	}
}

var errLine = regexp.MustCompile(`error\.error="([^"]+)"`)

// agentLastError finds the agent's most recent error for the session: first
// from opencode's log directory, then from stderr.
func agentLastError(sessionID, stderr string) string {
	home, _ := os.UserHomeDir()
	if d := lastErrorInLogs(filepath.Join(home, ".local/share/opencode/log"), sessionID); d != "" {
		return d
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.Contains(strings.ToLower(l), "error") {
			if len(l) > 300 {
				l = l[:300]
			}
			return l
		}
	}
	return ""
}

func lastErrorInLogs(dir, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	for _, f := range files {
		if found := scanLog(f, sessionID); found != "" {
			return found
		}
	}
	return ""
}

func scanLog(path, sessionID string) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()
	var found string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l := sc.Text()
		if strings.Contains(l, "level=ERROR") && strings.Contains(l, "session.id="+sessionID) {
			if m := errLine.FindStringSubmatch(l); m != nil {
				found = m[1]
			}
		}
	}
	return found
}
