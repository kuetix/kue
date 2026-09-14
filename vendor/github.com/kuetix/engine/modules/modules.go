package modules

import (
	di "github.com/kuetix/container"
	// _ "github.com/kuetix/engine/modules/billing/payment/transitions"
	// _ "github.com/kuetix/engine/modules/converse/transitions"
	// _ "github.com/kuetix/engine/modules/services/common/transitions"
)

func init() {
	di.Boot()
	// bootstrap.DependencyInjection[""] = func() {
	// }
}

func Enable() {
	// workflowsPaths := &defines.SafeMapString{}
	// workflowsPaths.Reset()
	// di.ToParameter("workflows_paths", workflowsPaths)
}
