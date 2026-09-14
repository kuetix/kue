module github.com/kuetix/kue

go 1.26.1

require (
	github.com/anare/filejsondb v1.0.0
	github.com/kuetix/container v0.1.0
	github.com/kuetix/engine v1.2.0
	github.com/kuetix/helpers v1.0.0
	github.com/kuetix/logger v1.0.0
	github.com/kuetix/social-oauth v0.0.0-00010101000000-000000000000
	github.com/kuetix/std-ai v0.0.0-20260713212628-e2f8e1f3ac13
	github.com/kuetix/std-auth v1.0.0
	github.com/kuetix/std-cli v1.0.0
	github.com/kuetix/std-decision v0.1.0
	github.com/kuetix/std-http v1.0.0
	github.com/kuetix/std-jsondb v0.0.0-20260909213749-631c78103419
	github.com/kuetix/std-mysql v0.0.0-20260909213754-5e914bec9fc1
	github.com/kuetix/std-push v0.1.0
	github.com/kuetix/std-redis v0.0.0-20260909213817-ab959ec8390b
	github.com/schollz/progressbar/v3 v3.19.1
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/term v0.46.0
	golang.org/x/text v0.42.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/asaskevich/EventBus v0.0.0-20200907212545-49d423059eef // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-sql-driver/mysql v1.9.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kuetix/std-core v1.0.0 // indirect
	github.com/kuetix/uuid v0.1.0 // indirect
	github.com/mark3labs/mcp-go v0.47.1 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/pnkj-kmr/simple-json-db v1.3.0 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	gopkg.in/ini.v1 v1.67.3 // indirect
)

replace github.com/kuetix/engine => ../engine

replace github.com/kuetix/std-core => ../packages/core

replace github.com/kuetix/std-cli => ../packages/cli

replace github.com/kuetix/std-auth => ../packages/auth

replace github.com/kuetix/std-http => ../packages/http

replace github.com/kuetix/std-ai => ../packages/ai

replace github.com/kuetix/std-decision => ../packages/decision

replace github.com/kuetix/std-jsondb => ../packages/jsondb

replace github.com/kuetix/std-mysql => ../packages/mysql

replace github.com/kuetix/std-push => ../packages/push

replace github.com/kuetix/std-redis => ../packages/redis

replace github.com/kuetix/social-oauth => ../packages/social-oauth
