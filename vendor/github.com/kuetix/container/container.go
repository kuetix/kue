package container

import (
	"fmt"
	"sync"
)

// ParametersMap Define the ParametersMap type
type ParametersMap map[string]interface{}

// FactoryFunc Define a specific type for the function stored in FactoryMap
type FactoryFunc func() interface{}

// SingletonMap Define the SingletonMap type
type SingletonMap map[string]interface{}

// FactoryMap Define the FactoryMap type
type FactoryMap map[string]FactoryFunc

// ParametersContainer Containers for parameters
var ParametersContainer ParametersMap

// SingletonContainer Containers for singletons
var SingletonContainer SingletonMap

// FactoryContainer Containers for factories
var FactoryContainer FactoryMap

// mutexes for thread safety
var parametersMu sync.RWMutex
var singletonMu sync.RWMutex
var factoryMu sync.RWMutex

func init() {
	ParametersContainer = make(ParametersMap)
	SingletonContainer = make(SingletonMap)
	FactoryContainer = make(FactoryMap)
}

// Get a function that searches both SingletonContainer and FactoryContainer
//
//goland:noinspection GoUnusedExportedFunction
func Get(key string) interface{} {
	// First, check the SingletonContainer
	singletonMu.RLock()
	value, exists := SingletonContainer[key]
	singletonMu.RUnlock()
	if exists {
		return value
	}

	// If not found in SingletonContainer, check the FactoryContainer
	factoryMu.RLock()
	factory, exists := FactoryContainer[key]
	factoryMu.RUnlock()
	if exists {
		return factory()
	}

	// If the key is not found in either container, return an error
	panic(fmt.Errorf("key %s not found in any container", key))
}

// Fetch retrieves a value from the SingletonContainer by key
func Fetch(key string) interface{} {
	singletonMu.RLock()
	value, exists := SingletonContainer[key]
	singletonMu.RUnlock()
	if !exists {
		panic(fmt.Errorf("singleton %s not found in any container", key))
	}
	return value
}

// Resolve retrieves a factory function from the FactoryContainer by key and invokes it
func Resolve(key string) interface{} {
	factoryMu.RLock()
	factory, exists := FactoryContainer[key]
	factoryMu.RUnlock()
	if !exists {
		panic(fmt.Errorf("factory %s not found in any container", key))
	}
	return factory()
}

// ToFetch sets a value in the SingletonContainer
func ToFetch(key string, value interface{}) {
	singletonMu.Lock()
	SingletonContainer[key] = value
	singletonMu.Unlock()
}

// ToFetchFunc sets a value in the SingletonContainer
//
//goland:noinspection GoUnusedExportedFunct
//goland:noinspection GoUnusedExportedFunction
func ToFetchFunc(key string, value FactoryFunc) {
	v := value()
	singletonMu.Lock()
	SingletonContainer[key] = v
	singletonMu.Unlock()
}

// ToResolve sets a factory function in the FactoryContainer
func ToResolve(key string, factory FactoryFunc) {
	factoryMu.Lock()
	FactoryContainer[key] = factory
	factoryMu.Unlock()
}

func ToParameter(key string, value interface{}) {
	parametersMu.Lock()
	ParametersContainer[key] = value
	parametersMu.Unlock()
}

func Parameter(key string) interface{} {
	parametersMu.RLock()
	value, exists := ParametersContainer[key]
	parametersMu.RUnlock()
	if !exists {
		panic(fmt.Errorf("parameter %s not found in any container", key))
	}

	return value
}

//goland:noinspection GoUnusedExportedFunction
func Has(name string) bool {
	singletonMu.RLock()
	_, exists := SingletonContainer[name]
	singletonMu.RUnlock()

	if !exists {
		factoryMu.RLock()
		_, exists = FactoryContainer[name]
		factoryMu.RUnlock()
	}

	if !exists {
		parametersMu.RLock()
		_, exists = ParametersContainer[name]
		parametersMu.RUnlock()
	}

	return exists
}

//goland:noinspection GoUnusedExportedFunction
func HasParameter(key string) bool {
	parametersMu.RLock()
	_, exists := ParametersContainer[key]
	parametersMu.RUnlock()

	return exists
}

//goland:noinspection GoUnusedExportedFunction
func CanFetch(name string) bool {
	singletonMu.RLock()
	_, exists := SingletonContainer[name]
	singletonMu.RUnlock()

	return exists
}

// CanResolve check is key exists in a FactoryContainer
func CanResolve(key string) bool {
	factoryMu.RLock()
	_, exists := FactoryContainer[key]
	factoryMu.RUnlock()

	return exists
}

// Reset clears all containers (intended for tests)
func Reset() {
	parametersMu.Lock()
	ParametersContainer = make(ParametersMap)
	parametersMu.Unlock()

	singletonMu.Lock()
	SingletonContainer = make(SingletonMap)
	singletonMu.Unlock()

	factoryMu.Lock()
	FactoryContainer = make(FactoryMap)
	factoryMu.Unlock()
}
