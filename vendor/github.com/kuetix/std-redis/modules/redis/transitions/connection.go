package transitions

import (
	"os"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

// ConnectionConfig holds the redis connection details, sourced entirely from
// environment variables: REDIS_ADDR, REDIS_PASSWORD, REDIS_DB. Falls back to
// "localhost:6379" / "" / 0 when unset.
type ConnectionConfig struct {
	Addr     string
	Password string
	DB       int
}

func newConnectionConfig() ConnectionConfig {
	cfg := ConnectionConfig{Addr: "localhost:6379"}

	if envAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR")); envAddr != "" {
		cfg.Addr = envAddr
	}
	cfg.Password = os.Getenv("REDIS_PASSWORD")
	if envDB := strings.TrimSpace(os.Getenv("REDIS_DB")); envDB != "" {
		if parsed, err := strconv.Atoi(envDB); err == nil {
			cfg.DB = parsed
		}
	}

	return cfg
}

// newClient builds a *redis.Client from the environment-sourced
// ConnectionConfig. The client dials lazily on first use, so this never
// blocks or fails at construction time.
func newClient() *goredis.Client {
	cfg := newConnectionConfig()
	return goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
