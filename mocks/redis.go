package mocks

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/mock"
)

type Redis struct {
	mock.Mock
}

func (m *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	args := m.Called(ctx, channels)
	return args.Get(0).(*redis.PubSub)
}

func (m *Redis) Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd {
	args := m.Called(ctx, channel, message)
	return args.Get(0).(*redis.IntCmd)
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
