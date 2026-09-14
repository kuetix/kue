package modules

import (
	di "github.com/kuetix/container"
	StdCoreModule "github.com/kuetix/std-core/modules"
	StdDecisionModule "github.com/kuetix/std-decision/modules"
)

func init() {
	di.Boot()
}

// Enable wires this package's push/* transitions plus everything it builds
// on: std-core (services/common/*, redis/*) and std-decision (decision/*,
// used by workflows/push/decide.wsl). The push/* transitions themselves are
// registered by this package's generated di.go init().
func Enable() {
	StdCoreModule.Enable()
	StdDecisionModule.Enable()
}
