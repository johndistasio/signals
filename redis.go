package main

import (
	"context"
	impl "github.com/go-redis/redis/v8"
	"time"
)

// Redis declares the methods we need from go-redis so that we can mock them for testing. It does not provide a complete
// abstraction as-is; the underlying implementation will leak out into the application and tests.
type Redis interface {
	Del(ctx context.Context, keys ...string) *impl.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *impl.BoolCmd
	Get(ctx context.Context, key string) *impl.StringCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *impl.BoolCmd
	Subscribe(ctx context.Context, channels ...string) *impl.PubSub
	Ping(ctx context.Context) *impl.StatusCmd
	Publish(ctx context.Context, channel string, message interface{}) *impl.IntCmd
	TTL(ctx context.Context, key string) *impl.DurationCmd
	ZRem(ctx context.Context, key string, members ...interface{}) *impl.IntCmd

	// Implements the 'scripter' interface from go-redis
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *impl.Cmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *impl.Cmd
	ScriptExists(ctx context.Context, hashes ...string) *impl.BoolSliceCmd
	ScriptLoad(ctx context.Context, script string) *impl.StringCmd
}

// PubSub declares the methods we need from go-redis's PubSub type so we can mock them for testing.
type PubSub interface {
	Close() error
	ReceiveMessage(ctx context.Context) (*impl.Message, error)
	Subscribe(ctx context.Context, channels ...string) error
}

func NewScript(src string) *impl.Script {
	return impl.NewScript(src)
}

func NewRedisClient() Redis {
	// TODO expose these and other options
	opts := &impl.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	}

	return impl.NewClient(opts)
}
