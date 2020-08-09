package main

import (
	"context"
	"errors"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

type MockRedis struct {
	mock.Mock
}

func (m *MockRedis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	args := m.Called(ctx, channels)
	return args.Get(0).(*redis.PubSub)
}

func (m *MockRedis) Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd {
	args := m.Called(ctx, channel, message)
	return args.Get(0).(*redis.IntCmd)
}

func (m *MockRedis) Eval(ctx context.Context, script string, keys []string, argv ...interface{}) *redis.Cmd {
	args := m.Called(ctx, script, keys, argv)
	return args.Get(0).(*redis.Cmd)
}

func (m *MockRedis) EvalSha(ctx context.Context, sha1 string, keys []string, argv ...interface{}) *redis.Cmd {
	args := m.Called(ctx, sha1, keys, argv)
	return args.Get(0).(*redis.Cmd)
}

func (m *MockRedis) ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd {
	args := m.Called(ctx, hashes)
	return args.Get(0).(*redis.BoolSliceCmd)
}

func (m *MockRedis) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	args := m.Called(ctx, script)
	return args.Get(0).(*redis.StringCmd)
}

func TestRedisRoomReceive(t *testing.T) {
	ch := make(chan *redis.Message, 1)
	defer close(ch)

	expected := "whoa"

	ch <- &redis.Message{Channel: "test", Pattern: "test", Payload: expected}

	room := &RedisRoom{name: "test", joined: true, ch: ch, rdb: new(MockRedis)}

	message, err := room.Receive(context.Background())

	if err != nil {
		t.Errorf("%v\n", err)
	}

	actual := string(message)

	if expected != actual {
		t.Errorf("expected %s got %s\n", expected, actual)
	}
}

func TestRedisRoomReceiveCancellation(t *testing.T) {
	// Set a timeout for overall test invocation.
	timeout, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan interface{})

	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Create a channel that will block receives forever.
		ch := make(chan *redis.Message)
		defer close(ch)

		room := &RedisRoom{name: "test", joined: true, ch: ch, rdb: new(MockRedis)}

		_, err := room.Receive(ctx)

		if err != context.Canceled {
			t.Errorf("%v\n", err)
		}
	}()

	select {
	case <-timeout.Done():
		t.Errorf("test invocation timed out")
	case <-done:
		return
	}
}

func TestRedisRoomPublish(t *testing.T) {
	cmd := redis.NewIntResult(1, nil)

	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	if err != nil {
		t.Errorf("expected %v, got %v\n", nil, err)
	}
}

func TestRedisRoomPublishError(t *testing.T) {
	cmd := redis.NewIntResult(0, errors.New("publish"))

	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	if err != ErrRoomGone {
		t.Errorf("expected %v, got %v\n", ErrRoomGone, err)
	}
}

func TestRedisRoomPublishCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := redis.NewIntResult(1, nil)
	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).WaitUntil(time.After(time.Second)).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(ctx, []byte{})

	if err != context.Canceled {
		t.Errorf("%v\n", err)
	}
}