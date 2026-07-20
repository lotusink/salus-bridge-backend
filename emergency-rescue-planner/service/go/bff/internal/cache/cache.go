package cache

import (
	"bff/internal/database"
	"sync"
	"time"
)

// TODO: replace this in-memory cache with Redis once a shared/multi-replica
// deployment is needed.

// cacheEntry wraps cached data with an expiration timestamp.
type cacheEntry struct {
	data      []database.GeoAreaRow
	expiresAt time.Time
}

// geoCache is an in-memory cache for GeoGroup query results.
// sync.Map is safe for concurrent read/write without explicit locking.
var geoCache sync.Map

// ReadGeoCache returns cached GeoAreaRow slice if the key exists and hasn't expired.
func ReadGeoCache(key string) ([]database.GeoAreaRow, bool) {
	// Mapping
	val, ok := geoCache.Load(key)
	if !ok {
		return nil, false
	}

	// Converting
	entry := val.(cacheEntry)

	// If expired
	if time.Now().After(entry.expiresAt) {
		geoCache.Delete(key)
		return nil, false
	}

	return entry.data, true
}

// WriteGeoCache stores a GeoAreaRow slice with the given TTL.
func WriteGeoCache(key string, data []database.GeoAreaRow, ttl time.Duration) {
	geoCache.Store(key, cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	})
}
