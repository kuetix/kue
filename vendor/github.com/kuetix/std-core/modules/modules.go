package modules

import (
	// _ "embed"

	di "github.com/kuetix/container"
	_ "github.com/kuetix/std-core"
)

// //go:embed "../modules.json"
// var modulesJson []byte
//
// //go:embed "../kuetix.json"
// var kuetixJson []byte

func init() {
	di.Boot()
}

func Enable() {
}
