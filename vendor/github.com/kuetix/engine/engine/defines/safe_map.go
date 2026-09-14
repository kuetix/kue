package defines

import (
	"sync"
)

type SafeMap struct {
	mu sync.RWMutex
	m  map[string]interface{}
}

// Reset safely resets the map
func (sm *SafeMap) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m = make(map[string]interface{})
}

// Set safely sets a key-value pair in the map
func (sm *SafeMap) Set(key, value string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

// Get safely retrieves a value for a given key
func (sm *SafeMap) Get(key string) (interface{}, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, exists := sm.m[key]
	return value, exists
}
