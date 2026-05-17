package modules

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/kue/modules/shared"
	StdCliModule "github.com/kuetix/std-cli/modules"
)

func init() {
	di.Boot()
}

//goland:noinspection GoUnusedExportedFunction
func Enable() {
	StdCliModule.Enable()
	if shared.TemplateManagerInstance == nil {
		_ = shared.InitializeTemplateManager("", "", "", "")
	}
}
