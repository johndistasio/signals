package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"testing"
	"time"
)

type mockRedis struct {}

func (r *mockRedis) Subscribe(context.Context, ...string) *redis.PubSub {return nil}
func (r *mockRedis) Publish(context.Context, string, interface{}) *redis.IntCmd {return redis.NewIntResult(0, nil)}
func (r *mockRedis) Eval(context.Context, string, []string, ...interface{}) *redis.Cmd {return nil}
func (r *mockRedis) EvalSha(context.Context, string, []string, ...interface{}) *redis.Cmd {return nil}
func (r *mockRedis) ScriptExists(context.Context, ...string) *redis.BoolSliceCmd {return nil}
func (r *mockRedis) ScriptLoad(context.Context, string) *redis.StringCmd {return nil}

type slowRedis struct {}

func (r *slowRedis) Subscribe(ctx context.Context, channels...string) *redis.PubSub {
	time.Sleep(10 * time.Second)
	return nil
}
func (r *slowRedis) Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd {
	time.Sleep(10 * time.Second)
	return redis.NewIntResult(0, nil)
}
func (r *slowRedis) Eval(context.Context, string, []string, ...interface{}) *redis.Cmd {return nil}
func (r *slowRedis) EvalSha(context.Context, string, []string, ...interface{}) *redis.Cmd {return nil}
func (r *slowRedis) ScriptExists(context.Context, ...string) *redis.BoolSliceCmd {return nil}
func (r *slowRedis) ScriptLoad(context.Context, string) *redis.StringCmd {return nil}

func TestRedisRoomPublish(t *testing.T) {
	room := &RedisRoom{name: "test", joined: true, rdb: &mockRedis{}}

	err := room.Publish(context.Background(), []byte{})

	if err != nil {
		t.Errorf("%v\n", err)
	}
}

func TestRedisRoomPublishCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	room := &RedisRoom{name: "test", joined: true, rdb: &slowRedis{}}

	err := room.Publish(ctx, []byte{})

	if err != context.Canceled {
		t.Errorf("%v\n", err)
	}
}

func TestRedisRoomReceive(t *testing.T) {
	ch := make(chan *redis.Message, 1)
	defer close(ch)

	expected := "whoa"

	ch <- &redis.Message{Channel: "test", Pattern: "test", Payload: expected}

	room := &RedisRoom{name: "test", joined: true, ch: ch, rdb: &mockRedis{}}

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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	room := &RedisRoom{name: "test", joined: true, rdb: &slowRedis{}}

	_, err := room.Receive(ctx)

	if err != context.Canceled {
		t.Errorf("%v\n", err)
	}
}