package adapter

import "sync"

// Constructor builds a fresh, unopened adapter instance.
type Constructor func() IDatabaseAdapter

var (
	registryMu sync.RWMutex
	registry   = map[string]Constructor{}
)

// Register associates an engine name with its constructor. Engine adapters call
// this from an init() function. Registering the same engine twice panics, since
// that indicates a programming error.
func Register(engine string, c Constructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[engine]; exists {
		panic("adapter: engine already registered: " + engine)
	}
	registry[engine] = c
}

// Engines returns the names of all registered engines.
func Engines() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
