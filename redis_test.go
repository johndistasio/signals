package main

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
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

type MockPubSub struct {
	mock.Mock
}

func (m *MockPubSub) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPubSub) ReceiveMessage(ctx context.Context) (*redis.Message, error) {
	args := m.Called(ctx)
	return args.Get(0).(*redis.Message), args.Error(1)
}

func (m *MockPubSub) Subscribe(ctx context.Context, channels ...string) error {
	args := m.Called(ctx, channels)
	return args.Error(0)
}

func TestRedisRoomJoin(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	room := &RedisRoom{name: "test", rdb: rdb}

	id := "test"

	timestamp, err := room.Enter(context.Background(), id, 1)
	assert.Nil(t, err)
	members, err := mr.ZMembers(room.Name())
	assert.Greater(t, len(members), 0)
	member := members[0]
	assert.Equal(t, id, member)
	assert.Nil(t, err)
	score, err := mr.ZScore(room.Name(), member)
	assert.Nil(t, err)
	assert.Equal(t, timestamp, int64(score))

	time.Sleep(1 * time.Second)

	id = "test2"

	timestamp, err = room.Enter(context.Background(), id, 1)
	assert.Nil(t, err)
	members, err = mr.ZMembers(room.Name())
	assert.Greater(t, len(members), 0)
	member = members[0]
	assert.Equal(t, id, member)
	assert.Nil(t, err)
	score, err = mr.ZScore(room.Name(), member)
	assert.Nil(t, err)
	assert.Equal(t, timestamp, int64(score))
}

func TestRedisRoomLeave(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	room := &RedisRoom{name: "test", rdb: rdb}

	id := "test"

	_, err = room.Enter(context.Background(), id, 1)
	assert.Nil(t, err)

	err = room.Leave(context.Background(), id)

	assert.Nil(t, err)

	members, err := mr.ZMembers(room.Name())
	assert.Equal(t, len(members), 0)
}

func TestRedisRoomReceive(t *testing.T) {
	expected := "whoa"

	pubsub := new(MockPubSub)
	pubsub.On("ReceiveMessage", mock.Anything).Return(&redis.Message{Payload: expected}, nil)

	room := &RedisRoom{name: "test", joined: true, pubsub: pubsub}

	m, err := room.Receive(context.Background())

	assert.Equal(t, []byte(expected), m)
	assert.Nil(t, err)

	/*
	ch := make(chan *redis.Message, 1)
	defer close(ch)

	expected := "whoa"

	ch <- &redis.Message{Channel: "test", Pattern: "test", Payload: expected}

	room := &RedisRoom{name: "test", joined: true, ch: ch, rdb: new(MockRedis)}

	message, err := room.Receive(context.Background())

	assert.Nil(t, err)
	assert.Equal(t, expected, string(message))

	 */
}

func TestRedisRoomReceiveCancellation(t *testing.T) {

	pubsub := new(MockPubSub)
	pubsub.On("ReceiveMessage", mock.Anything).Run(func (args mock.Arguments){
		// Not so long to hang the test forever, long enough to notice.
		time.Sleep(30 * time.Second)

	}).Return([]byte("whoa"), nil)

	room := &RedisRoom{name: "test", joined: true, pubsub: pubsub}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m, err := room.Receive(ctx)

	assert.Nil(t, m)
	assert.Equal(t, err, context.Canceled)

	/*

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

	 */
}

func TestRedisRoomPublish(t *testing.T) {
	cmd := redis.NewIntResult(1, nil)

	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	assert.Nil(t, err)
}

func TestRedisRoomPublishError(t *testing.T) {
	cmd := redis.NewIntResult(0, errors.New("publish"))

	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	assert.Equal(t, ErrRoomGone, err)
}

func TestRedisRoomPublishCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := redis.NewIntResult(1, nil)
	rdb := new(MockRedis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).WaitUntil(time.After(time.Second)).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(ctx, []byte{})

	assert.Equal(t, context.Canceled, err)
}