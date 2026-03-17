package cache

import (
	"context"
	"maps"
	"sync"
)

// InMemoryCache is a thread-safe in-memory cache backed by a map.
type InMemoryCache struct {
	mu    sync.RWMutex
	items map[string][]byte
}

// Compile-time interface check.
var _ Cache = (*InMemoryCache)(nil)

// NewInMemory creates a new in-memory cache.
func NewInMemory() *InMemoryCache {
	return &InMemoryCache{
		items: make(map[string][]byte, 256),
	}
}

// Get retrieves a value by key.
func (c *InMemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, ok := c.items[key]

	return val, ok, nil
}

// Set stores a value by key.
func (c *InMemoryCache) Set(_ context.Context, key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = value

	return nil
}

// GetMulti retrieves multiple values by keys.
func (c *InMemoryCache) GetMulti(_ context.Context, keys []string) (map[string][]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]byte, len(keys))
	for _, key := range keys {
		if val, ok := c.items[key]; ok {
			result[key] = val
		}
	}

	return result, nil
}

// SetMulti stores multiple key-value pairs.
func (c *InMemoryCache) SetMulti(_ context.Context, entries map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	maps.Copy(c.items, entries)

	return nil
}

// Close is a no-op for the in-memory cache.
func (c *InMemoryCache) Close() error {
	return nil
}
