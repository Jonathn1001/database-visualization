package connection

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// stubAdapter is a no-op adapter for manager tests.
type stubAdapter struct{ closed bool }

func (s *stubAdapter) Open(context.Context, model.ConnectionConfig) error { return nil }
func (s *stubAdapter) Close() error                                       { s.closed = true; return nil }
func (s *stubAdapter) Ping(context.Context) error                         { return nil }
func (s *stubAdapter) Engine() string                                     { return "stub" }
func (s *stubAdapter) Introspect(context.Context, adapter.IntrospectOptions) (*model.GraphModel, error) {
	return &model.GraphModel{}, nil
}
func (s *stubAdapter) Schemas(context.Context) ([]string, error) { return nil, nil }
func (s *stubAdapter) SampleData(context.Context, string, int) ([]map[string]any, error) {
	return nil, nil
}
func (s *stubAdapter) TableStats(context.Context, string) (*model.TableStats, error) { return nil, nil }
func (s *stubAdapter) ExplainQuery(context.Context, string) (*model.QueryPlan, error) {
	return nil, adapter.ErrOperationNotSupported
}
func (s *stubAdapter) SubscribeChanges(context.Context) (<-chan model.ChangeEvent, error) {
	return nil, adapter.ErrOperationNotSupported
}

func init() {
	adapter.Register("stub", func() adapter.IDatabaseAdapter { return &stubAdapter{} })
}

func TestNewIDFormat(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		id := newID()
		if !re.MatchString(id) {
			t.Fatalf("invalid UUID v4: %q", id)
		}
	}
}

func TestDetectEngine(t *testing.T) {
	cases := []struct {
		cfg  model.ConnectionConfig
		want string
		ok   bool
	}{
		{model.ConnectionConfig{Engine: "postgres"}, "postgres", true},
		{model.ConnectionConfig{DSN: "postgresql://u:p@h:5432/db"}, "postgres", true},
		{model.ConnectionConfig{DSN: "mongodb+srv://h/db"}, "mongodb", true},
		{model.ConnectionConfig{FilePath: "/tmp/x.db"}, "sqlite", true},
		{model.ConnectionConfig{DSN: "redis://h"}, "", false},
		{model.ConnectionConfig{}, "", false},
	}
	for _, c := range cases {
		got, err := DetectEngine(c.cfg)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("DetectEngine(%+v) = %q,%v; want %q", c.cfg, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("DetectEngine(%+v) expected error", c.cfg)
		}
	}
}

func TestManagerLifecycle(t *testing.T) {
	m := NewManager(Config{}, nil)
	mc, err := m.Open(context.Background(), model.ConnectionConfig{Engine: "stub", Password: "secret"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if mc.Config.Password != "" {
		t.Error("password not redacted in stored config")
	}
	got, err := m.Get(mc.ID)
	if err != nil || got != mc {
		t.Fatalf("Get returned %v, %v", got, err)
	}
	if err := m.Close(mc.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := m.Get(mc.ID); err == nil {
		t.Error("Get after Close should fail")
	}
}

func TestReaperClosesIdle(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 10 * time.Millisecond, ReapInterval: 5 * time.Millisecond}, nil)
	if _, err := m.Open(context.Background(), model.ConnectionConfig{Engine: "stub"}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.StartReaper(ctx)

	deadline := time.After(2 * time.Second)
	for {
		// Poll via List (does not Touch, so the idle timer is preserved).
		if len(m.List()) == 0 {
			return // reaped
		}
		select {
		case <-deadline:
			t.Fatal("idle connection was not reaped")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
