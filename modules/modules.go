package modules

import (
	di "github.com/kuetix/container"
	"github.com/kuetix/kue/modules/shared"
	SocialOauthModule "github.com/kuetix/social-oauth/modules"
	StdAiModule "github.com/kuetix/std-ai/modules"
	StdAuthModule "github.com/kuetix/std-auth/modules"
	StdCliModule "github.com/kuetix/std-cli/modules"
	StdDecisionModule "github.com/kuetix/std-decision/modules"
	StdHttpModule "github.com/kuetix/std-http/modules"
	StdJsondbModule "github.com/kuetix/std-jsondb/modules"
	StdMysqlModule "github.com/kuetix/std-mysql/modules"
	StdPushModule "github.com/kuetix/std-push/modules"
	StdRedisModule "github.com/kuetix/std-redis/modules"
)

func init() {
	di.Boot()
}

// Enable turns on every action namespace compiled into the kue binary. The
// full std library is linked in (not just std-cli) so `kue run` can execute
// real workflows offline — http calls, auth/jwt, ai steps, redis/jsondb/mysql
// stores, decision rules, push delivery, social-oauth, and all of
// services/common/*. `kue modules` / `kue transitions` report exactly what
// this list makes available.
//
//goland:noinspection GoUnusedExportedFunction
func Enable() {
	StdCliModule.Enable() // also pulls in std-core (services/common/*)
	StdAuthModule.Enable()
	StdHttpModule.Enable()
	StdAiModule.Enable()
	StdRedisModule.Enable()
	StdJsondbModule.Enable()
	StdMysqlModule.Enable()
	StdDecisionModule.Enable()
	StdPushModule.Enable() // builds on std-core + std-decision
	SocialOauthModule.Enable()
	if shared.TemplateManagerInstance == nil {
		_ = shared.InitializeTemplateManager("", "", "", "")
	}
}
