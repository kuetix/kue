package boot

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/logger"
)

func init() {
	logger.Debug("[engine] Booting services")
	di.Boot()
	di.DependencyInjection["services"] = func(name string) {
	}
}

func EnableServices() {
	logger.Debug("[engine] Enabling services")
}
