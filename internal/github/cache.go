package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethpandaops/panda/pkg/configpath"
)

const (
	// CacheFileName is the name of the update check cache file.
	CacheFileName = "update-check.json"

	// CacheTTL is how long a cached version check remains fresh.
	CacheTTL = 24 * time.Hour
)

// UpdateCache stores the result of a GitHub release check.
type UpdateCache struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// CachePath returns the full path to the update check cache file.
func CachePath() string {
	return filepath.Join(configpath.DefaultConfigDir(), CacheFileName)
}

// LoadCache reads the cache from disk. Returns nil, nil if the file
// does not exist.
func LoadCache() (*UpdateCache, error) {
	data, err := os.ReadFile(CachePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("reading update cache: %w", err)
	}

	var cache UpdateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("decoding update cache: %w", err)
	}

	return &cache, nil
}

// SaveCache writes the cache to disk atomically.
func SaveCache(cache *UpdateCache) error {
	path := CachePath()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("encoding update cache: %w", err)
	}

	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing update cache: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf("saving update cache: %w", err)
	}

	return nil
}

// IsStale returns true if the cache is older than CacheTTL.
func (c *UpdateCache) IsStale() bool {
	return time.Since(c.CheckedAt) > CacheTTL
}
