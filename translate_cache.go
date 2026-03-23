package pglike

import "sync"

// translateCache caches translated SQL strings to avoid repeated tokenization
// and translation passes for the same query.
type translateCache struct {
	mu      sync.RWMutex
	entries map[string]string
	order   []string // insertion order for LRU eviction
	maxSize int
}

var defaultCache = newTranslateCache(1000)

func newTranslateCache(maxSize int) *translateCache {
	return &translateCache{
		entries: make(map[string]string, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// get returns the cached translation, or empty string + false on miss.
func (c *translateCache) get(sql string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[sql]
	return v, ok
}

// put stores a translation, evicting the oldest entry if at capacity.
func (c *translateCache) put(sql, translated string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[sql]; exists {
		return
	}
	if len(c.entries) >= c.maxSize {
		// Evict oldest
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	c.entries[sql] = translated
	c.order = append(c.order, sql)
}

// TranslateCached translates PostgreSQL SQL to SQLite SQL, using a cache
// to avoid repeated translation of the same query.
func TranslateCached(sql string) (string, error) {
	if cached, ok := defaultCache.get(sql); ok {
		return cached, nil
	}
	translated, err := Translate(sql)
	if err != nil {
		return "", err
	}
	defaultCache.put(sql, translated)
	return translated, nil
}
