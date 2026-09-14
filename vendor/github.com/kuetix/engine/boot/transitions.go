package boot

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/logger"
)

func init() {
	di.Boot()
}

func EnableTransitions() {
	logger.Debug("[engine] Enabling transitions")
}
