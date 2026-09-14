package boot

import (
	"encoding/json"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
	"github.com/kuetix/uuid"
)

func init() {
	di.Boot()
	di.DependencyInjection["helpers"] = func(name string) {
		di.ToResolve(defines.HelpersPrefix+"bool", func() interface{} { return helpers.MustBool })
		di.ToResolve(defines.HelpersPrefix+"helpers.Id", func() interface{} { return uuid.Id })
		di.ToResolve(defines.HelpersPrefix+"helpers.EscapeRedisValue", func() interface{} { return helpers.EscapeRedisValue })
		di.ToResolve(defines.HelpersPrefix+"json.Marshal", func() interface{} { return json.Marshal })
	}
}

func EnableHelpers() {
	logger.Debug("[engine] Enabling helpers")
}
