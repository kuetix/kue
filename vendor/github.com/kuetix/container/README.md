# container

A tiny, thread-safe container package for Go that provides:
- Simple singleton storage
- Factory-based resolution (functions that build values on demand)
- A lightweight parameters store

Thread-safety: All registries are protected with `sync.RWMutex`. Concurrent reads and writes through the exported API are safe. Factories themselves must be safe if they mutate shared state.

## Table of contents
- Overview
- Installation
- Quick start
- Usage
  - Singletons (ToFetch/Fetch/ToFetchFunc)
  - Factories (ToResolve/Resolve)
  - Combined lookup (Get)
  - Parameters (ToParameter/Parameter)
  - Introspection helpers (Has, CanFetch, CanResolve, HasParameter)
- Error handling
- Thread-safety
- Testing
- Versioning and module path
- Contributing
- License

## Overview
This package exposes a minimal API around three registries (global to the package and guarded by RWMutexes):
- SingletonContainer: map of key -> concrete value
- FactoryContainer: map of key -> factory function returning a value
- ParametersContainer: map of key -> any parameter value

Lookups either return the stored value (for singletons/parameters) or call a registered factory to produce a value.

## Installation
This repository is a Go module. The module path is:

```
module github.com/kuetix/container
```

Install or update:

```
go get github.com/kuetix/container
```

Import in your code:

```go
import c "github.com/kuetix/container"
```

## Quick start
```go
package main

import (
    "fmt"
    c "github.com/kuetix/container"
)

func main() {
    // Register a singleton
    c.ToFetch("app.name", "demo-app")

    // Register a factory
    c.ToResolve("time.now", func() interface{} { return "2025-11-21T11:11:00Z" })

    // Read values
    fmt.Println(c.Fetch("app.name"))        // -> demo-app
    fmt.Println(c.Resolve("time.now"))      // -> calls the factory each time
    fmt.Println(c.Get("app.name"))          // -> works for either singleton or factory
}
```

## Usage

### Singletons (store once, read many)
- `ToFetch(key string, value interface{})`
- `ToFetchFunc(key string, value FactoryFunc)` — stores the result of calling the provided function immediately (eager), not the function itself.
- `Fetch(key string) interface{}` — panics if the key is missing.

Example:
```go
c.ToFetch("db", dbConn)
userRepo := NewUserRepo(c.Fetch("db").(DB))
```

### Factories (produce on demand)
- `ToResolve(key string, factory FactoryFunc)` — registers a factory.
- `Resolve(key string) interface{}` — calls the factory on each invocation; panics if missing.

Example:
```go
c.ToResolve("uuid", func() interface{} { return uuid.New() })
id := c.Resolve("uuid").(uuid.UUID)
```

### Combined lookup
- `Get(key string) interface{}` — looks in singletons first, then factories (invoking the factory if found); panics if nothing is registered for the key.

### Parameters
- `ToParameter(key string, value interface{})`
- `Parameter(key string) interface{}` — panics if missing.

Example:
```go
c.ToParameter("http.port", 8080)
port := c.Parameter("http.port").(int)
```

### Introspection helpers
- `Has(name string) bool` — true if key exists in any of the registries.
- `CanFetch(name string) bool` — true if a singleton exists.
- `CanResolve(key string) bool` — true if a factory exists.
- `HasParameter(key string) bool` — true if a parameter exists.

Example:
```go
if c.Has("db") { /* ... */ }
if c.CanResolve("uuid") { /* ... */ }
exists := c.HasParameter("http.port") // bool
```

## Error handling
Missing keys cause panics with informative error messages:
- `Fetch`, `Resolve`, `Get`, and `Parameter` will `panic(fmt.Errorf(...))` if the key is not found. Plan your usage accordingly or check with the helper predicates before calling.

## Thread-safety
All public operations are guarded by `sync.RWMutex`:
- Concurrent reads are allowed.
- Writes (registration) are serialized.
- Factories run outside locks; ensure your factory code is thread-safe when needed.

## Testing
Run tests:
```
go test ./...
```

Helpers:
- `Reset()` clears all registries and is intended for use in tests between cases.

## Versioning and module path
- Module path: `github.com/kuetix/container`.
- No Semantic Versioning tags are published yet.

## Contributing
- Issues and PRs are welcome.
- Please include tests for behavior changes.
- Keep the public API minimal and consistent.

## License
MIT License — see the `LICENSE` file for details.

Kuetix™ is an unregistered trademark of Anar Alishov. All rights reserved.
The Kuetix™ name and logo are not covered by this license.
