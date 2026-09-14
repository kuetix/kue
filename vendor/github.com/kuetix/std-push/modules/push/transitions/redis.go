package transitions

// Self-contained Redis access for the push package. Everything push does -
// subscription storage, per-user preferences, the bulk buffer, the
// cue:missed stream - lives in Redis, and the package deliberately does
// its own client construction from the same REDIS_* env every other
// kuetix binary reads rather than depending on std-redis, so the package
// (and its `make test`) stays self-contained: workflows/push/*.wsl only
// ever reference push/*, decision/* and services/common/* transitions.

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/cast"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// pushDebug reports whether PUSH_DEBUG is on - adds per-request detail
// (endpoint URL, VAPID JWT audience, APNs topic/host) to the [push:send]
// logs. Off by default; the summary lines log unconditionally.
func pushDebug() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PUSH_DEBUG"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// rdb builds a fresh client from REDIS_ADDR / REDIS_PASSWORD / REDIS_DB -
// identical resolution to zmist/protocol's newBroadcastClient and
// zmist/modules/user's db(). Callers Close() it.
func rdb() *redis.Client {
	db, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	return redis.NewClient(&redis.Options{
		Addr:     envOr("REDIS_ADDR", "127.0.0.1:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})
}

func bg() context.Context { return context.Background() }

// toStringSlice coerces a WSL-supplied argument (which arrives as
// []interface{}, a JSON-ish string, or a comma-joined string) into a
// clean []string. The cue:missed "already delivered" recipient list is
// passed as interface{}, never []string - see the WSL []string-param
// panic note in zmist's memory for why.
func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return dedupeNonEmpty(t)
	case string:
		s := strings.Trim(strings.TrimSpace(t), "[]")
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			out = append(out, strings.Trim(strings.TrimSpace(p), `"`))
		}
		return dedupeNonEmpty(out)
	default:
		return dedupeNonEmpty(cast.ToStringSlice(v))
	}
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
