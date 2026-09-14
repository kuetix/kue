package defines

import (
	"sync"
)

type SafeMapString struct {
	mu sync.RWMutex
	m  map[string]string
}

// Reset safely resets the map
func (sm *SafeMapString) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m = make(map[string]string)
}

// Set safely sets a key-value pair in the map
func (sm *SafeMapString) Set(key, value string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

// Get safely retrieves a value for a given key
func (sm *SafeMapString) Get(key string) (string, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, exists := sm.m[key]
	return value, exists
}
