// Package conformance provides a reusable test suite that every adapter
// implementation must pass (§9, §15.2). Adapter packages call RunSuite from
// their *_test.go files; this package is only ever imported by tests, so it is
// not linked into the production binary.
package conformance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// Options configures the conformance run for a specific engine.
type Options struct {
	// Config opens the adapter against a live test database.
	Config model.ConnectionConfig

	// AttemptWrite, if set, attempts a write through the open adapter's
	// underlying connection and returns the resulting error. The suite asserts
	// the error is non-nil — proving read-only enforcement (§22.1). If nil, the
	// read-only check is skipped.
	AttemptWrite func(ctx context.Context, a adapter.IDatabaseAdapter) error

	// ExplainQuery is a valid read-only query to pass to ExplainQuery. If empty,
	// the EXPLAIN check is skipped.
	ExplainQuery string

	// MinNodes is the number of nodes the test database is known to contain.
	// Used to exercise the MaxNodes cap meaningfully (must be >= 2).
	MinNodes int
}

// RunSuite runs the full conformance suite against adapters produced by factory.
func RunSuite(t *testing.T, factory func() adapter.IDatabaseAdapter, opts Options) {
	t.Helper()

	t.Run("OpenClosePing", func(t *testing.T) {
		a := open(t, factory, opts.Config)
		if err := a.Ping(context.Background()); err != nil {
			t.Fatalf("Ping after Open: %v", err)
		}
		if err := a.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("IntrospectNoIDCollisions", func(t *testing.T) {
		a := open(t, factory, opts.Config)
		defer a.Close()
		g, err := a.Introspect(context.Background(), adapter.IntrospectOptions{})
		if err != nil {
			t.Fatalf("Introspect: %v", err)
		}
		seen := map[string]bool{}
		for _, n := range g.Nodes {
			if seen[n.ID] {
				t.Errorf("duplicate node ID: %q", n.ID)
			}
			seen[n.ID] = true
		}
	})

	t.Run("IntrospectHonorsContextCancellation", func(t *testing.T) {
		a := open(t, factory, opts.Config)
		defer a.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled — Introspect must abort promptly

		start := time.Now()
		_, err := a.Introspect(ctx, adapter.IntrospectOptions{})
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("expected error from cancelled context, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		if elapsed > 2*time.Second {
			t.Errorf("introspect took %v to abort, want < 2s", elapsed)
		}
	})

	t.Run("IntrospectRespectsMaxNodes", func(t *testing.T) {
		if opts.MinNodes < 2 {
			t.Skip("MinNodes < 2; cannot exercise MaxNodes cap")
		}
		a := open(t, factory, opts.Config)
		defer a.Close()

		g, err := a.Introspect(context.Background(), adapter.IntrospectOptions{MaxNodes: 1})
		if err != nil {
			var apiErr *model.APIError
			if errors.As(err, &apiErr) && apiErr.Code == model.ErrSchemaTooLarge {
				return // acceptable outcome
			}
			t.Fatalf("expected SCHEMA_TOO_LARGE or capped result, got %v", err)
		}
		if len(g.Nodes) > 1 {
			t.Errorf("MaxNodes=1 but got %d nodes", len(g.Nodes))
		}
	})

	t.Run("ExplainNeverPanics", func(t *testing.T) {
		a := open(t, factory, opts.Config)
		defer a.Close()

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ExplainQuery panicked: %v", r)
			}
		}()

		q := opts.ExplainQuery
		if q == "" {
			q = "SELECT 1"
		}
		_, err := a.ExplainQuery(context.Background(), q)
		// Either a plan or ErrOperationNotSupported is acceptable; only a panic
		// fails this check.
		_ = err
	})

	t.Run("ReadOnlyEnforced", func(t *testing.T) {
		if opts.AttemptWrite == nil {
			t.Skip("no AttemptWrite hook provided")
		}
		a := open(t, factory, opts.Config)
		defer a.Close()
		if err := opts.AttemptWrite(context.Background(), a); err == nil {
			t.Error("expected write to fail under read-only adapter, got nil error")
		}
	})
}

func open(t *testing.T, factory func() adapter.IDatabaseAdapter, cfg model.ConnectionConfig) adapter.IDatabaseAdapter {
	t.Helper()
	a := factory()
	if err := a.Open(context.Background(), cfg); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return a
}
