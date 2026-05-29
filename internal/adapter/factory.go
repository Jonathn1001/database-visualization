package adapter

// New constructs an unopened adapter for the given engine. It returns
// ErrNotSupported if the engine has not been registered.
func New(engine string) (IDatabaseAdapter, error) {
	registryMu.RLock()
	c, ok := registry[engine]
	registryMu.RUnlock()
	if !ok {
		return nil, ErrNotSupported
	}
	return c(), nil
}
