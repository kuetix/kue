package filejsondb

import (
	"container/list"
	"sync"
)

const (
	defaultCacheMaxObjects = 1000000
	defaultCacheMaxBytes   = 0.5 * 1024 * 1024 * 1024
)

type cacheItem struct {
	key   string
	value []byte
	size  int
}

type CollectionCache struct {
	mu           sync.RWMutex
	items        map[string]*list.Element
	lru          *list.List
	maxObjects   int
	maxBytes     int
	currentBytes int
	fullSnapshot bool
}

func NewCollectionCache(maxObjects, maxBytes int) *CollectionCache {
	return &CollectionCache{
		items:      make(map[string]*list.Element),
		lru:        list.New(),
		maxObjects: maxObjects,
		maxBytes:   maxBytes,
	}
}

func (c *CollectionCache) purgeLocked() {
	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.currentBytes = 0
	c.fullSnapshot = false
}

func (c *CollectionCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeLocked()
}

func (c *CollectionCache) CalcSize(key string, value []byte) int {
	return len(key) + len(value)
}

func (c *CollectionCache) CloneBytes(v []byte) []byte {
	out := make([]byte, len(v))
	copy(out, v)
	return out
}

func (c *CollectionCache) RemoveElementLocked(el *list.Element) {
	item := el.Value.(*cacheItem)
	delete(c.items, item.key)
	c.currentBytes -= item.size
	c.lru.Remove(el)
}

func (c *CollectionCache) EnforceLimitsLocked() bool {
	evicted := false
	for (c.maxObjects > 0 && len(c.items) > c.maxObjects) || (c.maxBytes > 0 && c.currentBytes > c.maxBytes) {
		back := c.lru.Back()
		if back == nil {
			return evicted
		}
		c.RemoveElementLocked(back)
		evicted = true
		c.fullSnapshot = false
	}
	return evicted
}

func (c *CollectionCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := c.CalcSize(key, value)
	if c.maxBytes > 0 && size > c.maxBytes {
		if el, ok := c.items[key]; ok {
			c.RemoveElementLocked(el)
		}
		c.fullSnapshot = false
		return
	}

	if el, ok := c.items[key]; ok {
		item := el.Value.(*cacheItem)
		c.currentBytes -= item.size
		item.value = c.CloneBytes(value)
		item.size = size
		c.currentBytes += item.size
		c.lru.MoveToFront(el)
		c.EnforceLimitsLocked()
		return
	}

	item := &cacheItem{
		key:   key,
		value: c.CloneBytes(value),
		size:  size,
	}
	el := c.lru.PushFront(item)
	c.items[key] = el
	c.currentBytes += item.size
	c.EnforceLimitsLocked()
}

func (c *CollectionCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(el)
	item := el.Value.(*cacheItem)
	return c.CloneBytes(item.value), true
}

func (c *CollectionCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.RemoveElementLocked(el)
	}
}

func (c *CollectionCache) Warm(data map[string][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.purgeLocked()
	full := true
	for key, value := range data {
		size := c.CalcSize(key, value)
		if c.maxBytes > 0 && size > c.maxBytes {
			full = false
			continue
		}

		item := &cacheItem{
			key:   key,
			value: c.CloneBytes(value),
			size:  size,
		}
		el := c.lru.PushFront(item)
		c.items[key] = el
		c.currentBytes += item.size

		evicted := c.EnforceLimitsLocked()
		if evicted {
			full = false
		}
	}
	c.fullSnapshot = full
}

func (c *CollectionCache) GetAllIfComplete() (map[string][]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.fullSnapshot {
		return nil, false
	}

	out := make(map[string][]byte, len(c.items))
	for key, el := range c.items {
		item := el.Value.(*cacheItem)
		out[key] = c.CloneBytes(item.value)
	}

	return out, true
}
