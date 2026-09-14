package transitions

// Redis Streams helpers for the cue:missed pipeline stage. A trimmed,
// self-contained port of zmist/protocol/streams.go's XAdd + ConsumeGroup
// (kept byte-compatible with the other Cue stages: XADD field map, a
// consumer group created MkStream, one XACK per entry whether the handler
// succeeded or not, NOGROUP self-heal). No dead-letter/XCLAIM - same
// documented gap as every other stage.
//
// Retention: XACK only clears an entry from the consumer group's pending
// list - Redis keeps the entry in the stream forever, and nothing here
// ever XDELs. So streamXAdd caps the stream with an approximate MAXLEN on
// every append (PUSH_MISSED_STREAM_MAXLEN, default 20000, 0 disables) to
// stop an unbounded backlog. consumeGroup logs the live backlog
// (XLEN + group pending/lag) once per non-empty batch.

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultMissedStreamMaxLen = 20000

// missedStreamMaxLen is the approximate cap XADD trims cue:missed to.
func missedStreamMaxLen() int64 {
	return int64(envInt("PUSH_MISSED_STREAM_MAXLEN", defaultMissedStreamMaxLen))
}

// streamXAdd appends one entry to stream with the given fields, trimming
// the stream to an approximate MAXLEN so XACKed-but-undeleted entries
// can't accumulate without bound.
func streamXAdd(ctx context.Context, stream string, fields map[string]interface{}) error {
	client := rdb()
	defer client.Close()
	args := &redis.XAddArgs{Stream: stream, Values: fields}
	if maxLen := missedStreamMaxLen(); maxLen > 0 {
		args.MaxLen = maxLen
		args.Approx = true
	}
	return client.XAdd(ctx, args).Err()
}

// streamStats reports the current backlog for stream/group: total entries
// still held in the stream (XLEN) and the group's not-yet-acked count and
// lag (undelivered entries). Best-effort - any error yields the zero
// values so callers can log unconditionally.
func streamStats(ctx context.Context, client *redis.Client, stream, group string) (length, pending, lag int64) {
	length, _ = client.XLen(ctx, stream).Result()
	groups, err := client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.Name == group {
			return length, g.Pending, g.Lag
		}
	}
	return
}

// consumeGroup blocks forever, reading stream via a consumer group and
// calling handler once per entry.
func consumeGroup(ctx context.Context, stream, group, consumer string, handler func(id string, fields map[string]string) error) error {
	client := rdb()
	defer client.Close()

	if err := ensureGroup(ctx, client, stream, group); err != nil {
		return err
	}

	for {
		result, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    10,
			Block:    0,
		}).Result()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("[push:stream] XReadGroup(%s/%s) error: %v", stream, group, err)
			if isNoGroup(err) {
				if recreateErr := ensureGroup(ctx, client, stream, group); recreateErr == nil {
					log.Printf("[push:stream] recreated missing stream/group %s/%s", stream, group)
				}
			}
			time.Sleep(time.Second)
			continue
		}

		processed, failed := 0, 0
		for _, s := range result {
			for _, msg := range s.Messages {
				fields := make(map[string]string, len(msg.Values))
				for k, v := range msg.Values {
					if sv, ok := v.(string); ok {
						fields[k] = sv
					}
				}
				if herr := handler(msg.ID, fields); herr != nil {
					failed++
					log.Printf("[push:stream] handler error for %s %s: %v", stream, msg.ID, herr)
				}
				processed++
				if ackErr := client.XAck(ctx, stream, group, msg.ID).Err(); ackErr != nil {
					log.Printf("[push:stream] XAck(%s/%s/%s) error: %v", stream, group, msg.ID, ackErr)
				}
			}
		}
		if processed > 0 {
			length, pending, lag := streamStats(ctx, client, stream, group)
			log.Printf("[push:stream] consumed batch stream=%s group=%s processed=%d failed=%d backlog{xlen=%d pending=%d lag=%d}",
				stream, group, processed, failed, length, pending, lag)
		}
	}
}

func isBusyGroup(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "BUSYGROUP")
}

func isNoGroup(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "NOGROUP")
}

func ensureGroup(ctx context.Context, client *redis.Client, stream, group string) error {
	if err := client.XGroupCreateMkStream(ctx, stream, group, "$").Err(); err != nil && !isBusyGroup(err) {
		return err
	}
	return nil
}
