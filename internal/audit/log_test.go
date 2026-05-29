package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLoggerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer l.Close()

	l.Record(Entry{ConnectionID: "c1", Engine: "postgres", Operation: OpExplain, SQL: "SELECT 1", DurationMS: 5, RowCount: 1})
	l.Record(Entry{ConnectionID: "c1", Engine: "postgres", Operation: OpSample, Error: ErrString(errors.New("boom"))})

	day := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(dir, "audit-"+day+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit file: %v", err)
	}
	defer f.Close()

	var n int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %d not valid JSON: %v", n, err)
		}
		if e.TS == "" {
			t.Errorf("entry %d missing ts", n)
		}
		n++
	}
	if n != 2 {
		t.Errorf("got %d entries, want 2", n)
	}
}

func TestPruneRemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "audit-2000-01-01.jsonl")
	recent := filepath.Join(dir, "audit-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	for _, p := range []string{old, recent} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	l := &fileLogger{dir: dir}
	l.prune(time.Now())

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old audit file should have been pruned")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("recent audit file should remain")
	}
}

func TestNopLogger(t *testing.T) {
	var l Logger = Nop{}
	l.Record(Entry{Operation: OpStats})
	if err := l.Close(); err != nil {
		t.Errorf("Nop.Close: %v", err)
	}
}
