package resolver

import (
	"sync"
	"time"

	goversion "github.com/hashicorp/go-version"
)

const (
	versionCacheTTL   = 6 * time.Hour
	changelogCacheTTL = 24 * time.Hour
)

type cacheEntry struct {
	versions  []*goversion.Version
	expiresAt time.Time
}

// Cache provides in-memory caching for registry responses.
type Cache struct {
	mu       sync.RWMutex
	versions map[string]cacheEntry
}

// NewCache creates a new in-memory cache.
func NewCache() *Cache {
	return &Cache{
		versions: make(map[string]cacheEntry),
	}
}

// GetVersions retrieves cached versions for a key.
func (c *Cache) GetVersions(key string) ([]*goversion.Version, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.versions[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.versions, true
}

// SetVersions stores versions in the cache.
func (c *Cache) SetVersions(key string, versions []*goversion.Version) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.versions[key] = cacheEntry{
		versions:  versions,
		expiresAt: time.Now().Add(versionCacheTTL),
	}
}
