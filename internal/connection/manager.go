// Package connection manages the lifecycle of open database connections: an
// in-memory pool keyed by server-generated UUID, with an idle reaper (§7).
package connection

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/elgnas/dbviz/internal/adapter"
	"github.com/elgnas/dbviz/internal/model"
)

// State is a connection's lifecycle state (§7.2).
type State string

const (
	StateOpening  State = "opening"
	StateOpen     State = "open"
	StateIdle     State = "idle"
	StateDegraded State = "degraded"
	StateClosed   State = "closed"
)

// Config tunes the manager. Zero values fall back to defaults in Manager.
type Config struct {
	IdleTimeout  time.Duration // close connections idle longer than this
	ReapInterval time.Duration // how often the reaper runs
}

func (c Config) withDefaults() Config {
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 30 * time.Minute
	}
	if c.ReapInterval <= 0 {
		c.ReapInterval = 60 * time.Second
	}
	return c
}

// ManagedConnection wraps an adapter with lifecycle metadata. The password in
// Config is always redacted (§22.7).
type ManagedConnection struct {
	ID       string
	Engine   string
	Config   model.ConnectionConfig
	Adapter  adapter.IDatabaseAdapter
	OpenedAt time.Time

	mu         sync.Mutex
	state      State
	lastUsedAt time.Time
}

// Touch marks the connection as recently used, resetting the idle timer.
func (c *ManagedConnection) Touch() {
	c.mu.Lock()
	c.lastUsedAt = time.Now()
	if c.state == StateIdle {
		c.state = StateOpen
	}
	c.mu.Unlock()
}

// State returns the current lifecycle state.
func (c *ManagedConnection) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *ManagedConnection) setState(s State) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
}

func (c *ManagedConnection) idleFor() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.lastUsedAt)
}

// Manager is the in-memory connection pool.
type Manager struct {
	cfg Config
	log *slog.Logger

	mu    sync.RWMutex
	conns map[string]*ManagedConnection
}

// NewManager constructs a Manager.
func NewManager(cfg Config, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{
		cfg:   cfg.withDefaults(),
		log:   log,
		conns: map[string]*ManagedConnection{},
	}
}

// Open detects the engine, opens an adapter, and registers it. The returned
// ManagedConnection carries a redacted config.
func (m *Manager) Open(ctx context.Context, cfg model.ConnectionConfig) (*ManagedConnection, error) {
	engine, err := DetectEngine(cfg)
	if err != nil {
		return nil, err
	}
	a, err := adapter.New(engine)
	if err != nil {
		return nil, model.NewAPIError(model.ErrAdapterNotSupported, "engine not supported: "+engine)
	}
	if err := a.Open(ctx, cfg); err != nil {
		return nil, err
	}

	now := time.Now()
	mc := &ManagedConnection{
		ID:         newID(),
		Engine:     engine,
		Config:     cfg.Redacted(),
		Adapter:    a,
		OpenedAt:   now,
		state:      StateOpen,
		lastUsedAt: now,
	}
	m.mu.Lock()
	m.conns[mc.ID] = mc
	m.mu.Unlock()

	m.log.Info("connection opened", "id", mc.ID, "engine", engine)
	return mc, nil
}

// Get returns the connection by ID and marks it used, or ErrConnNotFound.
func (m *Manager) Get(id string) (*ManagedConnection, error) {
	m.mu.RLock()
	mc, ok := m.conns[id]
	m.mu.RUnlock()
	if !ok {
		return nil, model.NewAPIError(model.ErrConnNotFound, "connection not found").
			WithHint("the session may have expired; reconnect")
	}
	mc.Touch()
	return mc, nil
}

// List returns all managed connections.
func (m *Manager) List() []*ManagedConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ManagedConnection, 0, len(m.conns))
	for _, c := range m.conns {
		out = append(out, c)
	}
	return out
}

// Close closes and removes a connection by ID.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	mc, ok := m.conns[id]
	if ok {
		delete(m.conns, id)
	}
	m.mu.Unlock()
	if !ok {
		return model.NewAPIError(model.ErrConnNotFound, "connection not found")
	}
	mc.setState(StateClosed)
	m.log.Info("connection closed", "id", id)
	return mc.Adapter.Close()
}

// CloseAll closes every connection. Used on server shutdown.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	conns := m.conns
	m.conns = map[string]*ManagedConnection{}
	m.mu.Unlock()
	for _, mc := range conns {
		mc.setState(StateClosed)
		_ = mc.Adapter.Close()
	}
	return nil
}

// StartReaper runs a background loop that closes idle connections. It returns
// when ctx is cancelled.
func (m *Manager) StartReaper(ctx context.Context) {
	t := time.NewTicker(m.cfg.ReapInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.reapIdle()
		}
	}
}

func (m *Manager) reapIdle() {
	var stale []string
	m.mu.RLock()
	for id, mc := range m.conns {
		if mc.idleFor() > m.cfg.IdleTimeout {
			stale = append(stale, id)
		}
	}
	m.mu.RUnlock()
	for _, id := range stale {
		m.log.Info("reaping idle connection", "id", id)
		_ = m.Close(id)
	}
}
