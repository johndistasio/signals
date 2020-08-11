package main

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/johndistasio/signaling/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

func TestRedisRoomJoin(t *testing.T) {
	mr, err := miniredis.Run()

	if err != nil {
		panic(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	defer func() {
		mr.Close()
		_ = rdb.Close()
	}()

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

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	defer func() {
		mr.Close()
		_ = rdb.Close()
	}()

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

	pubsub := new(mocks.PubSub)
	pubsub.On("ReceiveMessage", mock.Anything).Return(&redis.Message{Payload: expected}, nil)

	room := &RedisRoom{name: "test", joined: true, pubsub: pubsub}

	m, err := room.Receive(context.Background())

	assert.Equal(t, []byte(expected), m)
	assert.Nil(t, err)
}

func TestRedisRoomReceiveCancellation(t *testing.T) {
	pubsub := new(mocks.PubSub)
	pubsub.On("ReceiveMessage", mock.Anything).Run(func(args mock.Arguments) {
		// Not so long to hang the test forever, long enough to notice.
		time.Sleep(30 * time.Second)

	}).Return([]byte("whoa"), nil)

	room := &RedisRoom{name: "test", joined: true, pubsub: pubsub}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m, err := room.Receive(ctx)

	assert.Nil(t, m)
	assert.Equal(t, err, context.Canceled)
}

func TestRedisRoomPublish(t *testing.T) {
	cmd := redis.NewIntResult(1, nil)

	rdb := new(mocks.Redis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	assert.Nil(t, err)
}

func TestRedisRoomPublishError(t *testing.T) {
	cmd := redis.NewIntResult(0, errors.New("publish"))

	rdb := new(mocks.Redis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(context.Background(), []byte{})

	assert.Equal(t, ErrRoomGone, err)
}

func TestRedisRoomPublishCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := redis.NewIntResult(1, nil)
	rdb := new(mocks.Redis)
	rdb.On("Publish", mock.Anything, mock.Anything, mock.Anything).WaitUntil(time.After(time.Second)).Return(cmd)

	room := &RedisRoom{name: "test", joined: true, rdb: rdb}

	err := room.Publish(ctx, []byte{})

	assert.Equal(t, context.Canceled, err)
}
