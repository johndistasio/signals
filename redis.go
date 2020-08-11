package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"time"
)

// Redis declares the methods we need from go-redis so that we can mock them for testing.
type Redis interface {
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd

	// Implements the 'scripter' interface from go-redis
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd
	ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd
	ScriptLoad(ctx context.Context, script string) *redis.StringCmd
}

// PubSub declares the methods we need from go-redis's PubSub type so we can mock them for testing.
type PubSub interface {
	Close() error
	ReceiveMessage(ctx context.Context) (*redis.Message, error)
	Subscribe(ctx context.Context, channels ...string) error
}