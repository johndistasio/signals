package mocks

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/mock"
	"time"
)

type Redis struct {
	mock.Mock
}

func (m *Redis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	args := m.Called(ctx, keys)
	return args.Get(0).(*redis.IntCmd)
}

func (m *Redis) Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	args := m.Called(ctx, key, expiration)
	return args.Get(0).(*redis.BoolCmd)
}

func (m *Redis) Get(ctx context.Context, key string) *redis.StringCmd{
	args := m.Called(ctx, key)
	return args.Get(0).(*redis.StringCmd)
}

func (m *Redis) Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd {
	args := m.Called(ctx, channel, message)
	return args.Get(0).(*redis.IntCmd)
}

func (m *Redis) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	args := m.Called(ctx, key, value, expiration)
	return args.Get(0).(*redis.BoolCmd)
}

func (m *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	args := m.Called(ctx, channels)
	return args.Get(0).(*redis.PubSub)
}

func (m *Redis) TTL(ctx context.Context, key string) *redis.DurationCmd {
	args := m.Called(ctx, key)
	return args.Get(0).(*redis.DurationCmd)
}

func (m *Redis) Eval(ctx context.Context, script string, keys []string, argv ...interface{}) *redis.Cmd {
	args := m.Called(ctx, script, keys, argv)
	return args.Get(0).(*redis.Cmd)
}

func (m *Redis) EvalSha(ctx context.Context, sha1 string, keys []string, argv ...interface{}) *redis.Cmd {
	args := m.Called(ctx, sha1, keys, argv)
	return args.Get(0).(*redis.Cmd)
}

func (m *Redis) ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd {
	args := m.Called(ctx, hashes)
	return args.Get(0).(*redis.BoolSliceCmd)
}

func (m *Redis) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	args := m.Called(ctx, script)
	return args.Get(0).(*redis.StringCmd)
}

type PubSub struct {
	mock.Mock
}

func (m *PubSub) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *PubSub) ReceiveMessage(ctx context.Context) (*redis.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(*redis.Message), args.Error(1)
}

func (m *PubSub) Subscribe(ctx context.Context, channels ...string) error {
	args := m.Called(ctx, channels)
	return args.Error(0)
}
