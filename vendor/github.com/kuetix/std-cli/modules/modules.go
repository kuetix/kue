package modules

import (
	di "github.com/kuetix/container"
	_ "github.com/kuetix/std-cli/modules/cli/helpers"
	_ "github.com/kuetix/std-cli/modules/cli/transitions"
	StdCoreModule "github.com/kuetix/std-core/modules"
)

func init() {
	di.Boot()
}

func Enable() {
	StdCoreModule.Enable()
}
