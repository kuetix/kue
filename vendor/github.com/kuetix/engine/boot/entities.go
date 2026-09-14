package boot

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/logger"
)

func init() {
	di.Boot()
	di.DependencyInjection["entities"] = func(name string) {
	}
}

func EnableEntities() {
	logger.Debug("[engine] Enabling entities")
}
