package boot

import (
	"github.com/kuetix/logger"
)

func EnableBoot() {
	logger.Debugf("[engine] Enabling services")
	EnableEntities()
	EnableHelpers()
	EnableServices()
	EnableTransitions()
}
