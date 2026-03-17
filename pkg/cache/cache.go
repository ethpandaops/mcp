// Package cache provides a generic key-value cache interface.
package cache

import "context"

// Cache is a generic key-value cache.
type Cache interface {
	// Get retrieves a value by key. Returns the value, whether it was found, and any error.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores a value by key.
	Set(ctx context.Context, key string, value []byte) error

	// GetMulti retrieves multiple values by keys. Returns a map of found key-value pairs.
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

	// SetMulti stores multiple key-value pairs.
	SetMulti(ctx context.Context, entries map[string][]byte) error

	// Close releases resources held by the cache.
	Close() error
}
