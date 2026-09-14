# kuetix/uuid

Small utilities around UUID generation and formatting built on top of `github.com/google/uuid`.

[![CI](https://github.com/kuetix/uuid/actions/workflows/ci.yml/badge.svg)](https://github.com/kuetix/uuid/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/kuetix/uuid.svg)](https://pkg.go.dev/github.com/kuetix/uuid)
[![Go Report Card](https://goreportcard.com/badge/github.com/kuetix/uuid)](https://goreportcard.com/report/github.com/kuetix/uuid)

## Install
```
go get github.com/kuetix/uuid
```

## Usage
```go
package main

import (
    "fmt"
    kuuid "github.com/kuetix/uuid"
)

func main() {
    // Deterministic UUIDv5 derived from an identity string
    u := kuuid.IdV5("example")
    fmt.Println("uuid:", u.String()) // canonical XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX

    // Canonical string form
    fmt.Println("UUId:", kuuid.UUId("example"))

    // Dashless hex (32 chars)
    fmt.Println("Id:", kuuid.Id("example"))

    // Base64 encoding of the 16-byte UUID
    fmt.Println("Base64:", kuuid.Base64Id("example"))

    // Remove dashes from a canonical UUID string
    fmt.Println("UId:", kuuid.UId(u.String()))
}
```

## Development
- Minimal Go module with tests under `test/`.
- Run all tests:
```
go test ./...
```

See `CONTRIBUTING.md` for details.

## License
MIT License — see the `LICENSE` file for details.

Kuetix™ is an unregistered trademark of Anar Alishov. All rights reserved.
The Kuetix™ name and logo are not covered by this license.
