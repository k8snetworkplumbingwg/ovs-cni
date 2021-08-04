package cache

import (
	"time"
)

// Cache is the ovs bridges cache
type Cache struct {
	lastRefreshTime time.Time
	bridges         map[string]bool
}

// Refresh updates the cached bridges and refresh time
func (c *Cache) Refresh(freshBridges map[string]bool) {
	c.bridges = freshBridges
	c.lastRefreshTime = time.Now()
}

// LastRefreshTime returns the last time the cache was updated
func (c *Cache) LastRefreshTime() time.Time {
	return c.lastRefreshTime
}

// Bridges return the cached bridges
func (c Cache) Bridges() map[string]bool {
	bridgesCopy := make(map[string]bool)
	for bridge, exist := range c.bridges {
		bridgesCopy[bridge] = exist
	}
	return bridgesCopy
}
