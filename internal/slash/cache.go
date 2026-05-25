package slash

import "sync"

// Cache is a goroutine-safe map cache used by the registry to memoize
// expensive list views (All, UserInvocable, ModelInvocable). The registry
// invalidates the cache on every mutation; callers should not store values
// expected to outlive a single read cycle.
type Cache struct {
	mu      sync.RWMutex
	entries map[string][]*Command
}

// NewCache returns an empty Cache ready for use.
func NewCache() *Cache {
	return &Cache{entries: make(map[string][]*Command)}
}

// Get returns the cached slice for key and reports whether it was present.
// The returned slice must not be mutated by the caller.
func (c *Cache) Get(key string) ([]*Command, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cmds, ok := c.entries[key]
	return cmds, ok
}

// Set stores cmds under key, replacing any existing entry.
func (c *Cache) Set(key string, cmds []*Command) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cmds
}

// Invalidate clears every cached entry. Subsequent Get calls miss.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string][]*Command)
}

// InvalidateKey clears the entry stored under key, if any.
func (c *Cache) InvalidateKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
