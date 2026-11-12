package generic

import (
	"sync"
	"time"
)

type entry[V any] struct {
	mu sync.RWMutex
	v  V
}

type Cache[K comparable, V any] struct {
	entry[V]
	key K
	ttl time.Duration
}

func New[K comparable, V any]() Cache[K, V] { return Cache[K, V]{} }

func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key, c.v, c.ttl = key, value, ttl
}

func (c *Cache[K, V]) Get() (K, V, time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.key, c.v, c.ttl
}
