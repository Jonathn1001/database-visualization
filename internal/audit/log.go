// Package audit records every query dbviz sends to a user database, separate
// from application logs (§18). Entries are JSON Lines, rotated daily and pruned
// after 90 days.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Operation identifies the kind of database access being recorded.
const (
	OpIntrospect = "introspect"
	OpExplain    = "explain"
	OpSample     = "sample"
	OpStats      = "stats"
)

const retentionDays = 90

// Entry is one audit record (§18.2). The SQL field must never contain
// interpolated parameter values.
type Entry struct {
	TS           string  `json:"ts"`
	ConnectionID string  `json:"connection_id"`
	Engine       string  `json:"engine"`
	Database     string  `json:"database"`
	Operation    string  `json:"operation"`
	SQL          string  `json:"sql"`
	DurationMS   int64   `json:"duration_ms"`
	RowCount     int     `json:"row_count"`
	Error        *string `json:"error"`
}

// Logger records audit entries.
type Logger interface {
	Record(e Entry)
	Close() error
}

// Nop is a no-op logger used when auditing is disabled.
type Nop struct{}

func (Nop) Record(Entry) {}
func (Nop) Close() error { return nil }

// fileLogger appends JSONL to a daily file under dir.
type fileLogger struct {
	dir string

	mu  sync.Mutex
	day string
	f   *os.File
}

// New returns a file-backed audit logger writing under dir. If dir is empty it
// defaults to ~/.dbviz/audit. Old files (> 90 days) are pruned on startup.
func New(dir string) (Logger, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".dbviz", "audit")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &fileLogger{dir: dir}
	l.prune(time.Now())
	return l, nil
}

// DefaultDir returns the standard audit directory path.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dbviz", "audit"), nil
}

func (l *fileLogger) Record(e Entry) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureFile(time.Now()); err != nil {
		return
	}
	_, _ = l.f.Write(append(line, '\n'))
}

// ensureFile opens (or rotates to) the file for the current day. Caller holds mu.
func (l *fileLogger) ensureFile(now time.Time) error {
	day := now.UTC().Format("2006-01-02")
	if l.f != nil && l.day == day {
		return nil
	}
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
	path := filepath.Join(l.dir, fmt.Sprintf("audit-%s.jsonl", day))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	l.f = f
	l.day = day
	return nil
}

func (l *fileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		err := l.f.Close()
		l.f = nil
		return err
	}
	return nil
}

// prune removes audit files older than the retention window.
func (l *fileLogger) prune(now time.Time) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dayStr := strings.TrimSuffix(strings.TrimPrefix(name, "audit-"), ".jsonl")
		day, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			continue
		}
		if day.Before(cutoff) {
			_ = os.Remove(filepath.Join(l.dir, name))
		}
	}
}

// ErrString returns a pointer to err's message, or nil if err is nil — for the
// Entry.Error field.
func ErrString(err error) *string {
	if err == nil {
		return nil
	}
	s := err.Error()
	return &s
}
