package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"sync"
	"time"
)

// Redis declares the methods we need from go-redis so that we can mock them for testing.
type Redis interface {
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd


	// Implements the 'scripter' interface from go-redis
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd
	ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd
	ScriptLoad(ctx context.Context, script string) *redis.StringCmd
}

// RedisRoom is a Room implementation that uses Redis as both a lock manager and a message bus.
type RedisRoom struct{
	name string
	rdb  Redis
	mu sync.Mutex
	joined bool
	pubsub *redis.PubSub
	ch <-chan *redis.Message
}

func (r *RedisRoom) Name() string {
	return r.name
}

const roomKeyPrefix = "room:"

const roomSize = 2

func (r *RedisRoom) key() string {
	return roomKeyPrefix + r.name
}

func (r *RedisRoom) Join(ctx context.Context, id string, t time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.acquire(ctx, id, t); err != nil {
		return err
	}

	if r.joined {
		return nil
	}

	r.pubsub = r.rdb.Subscribe(ctx, r.key())
	r.ch = r.pubsub.Channel()

	r.joined = true

	return nil
}

// todo document arguments
var acquireScript = redis.NewScript(`
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])

	if redis.call('ZCARD', KEYS[1]) < tonumber(ARGV[2]) then
		redis.call('ZADD', KEYS[1], ARGV[3], ARGV[4])
		return 1
    elseif redis.call('ZSCORE', KEYS[1], ARGV[4]) then
		redis.call('ZADD', KEYS[1], ARGV[3], ARGV[4])
		return 1
	else
		return 0
	end
`)


// TODO how do we test this... make script a param?

func (r *RedisRoom) acquire(ctx context.Context, id string, t time.Duration) error {
	now := time.Now()
	then := now.Add(t)

	acq, err := acquireScript.Run(ctx, r.rdb, []string{r.key()}, then.Unix(), roomSize, now.Unix(), id).Result()

	if err != nil {
		return ErrRoomGone
	}

	if acq != int64(1) {
		return ErrRoomFull
	}

	return nil
}

var releaseScript = redis.NewScript("return redis.call('ZREM', KEYS[1], ARGV[1])")

func (r *RedisRoom) release(ctx context.Context, id string) error {
	err := releaseScript.Run(ctx, r.rdb, []string{r.key()}, id).Err()

	if err != nil {
		return ErrRoomGone
	}

	return nil
}


func (r *RedisRoom) Leave(ctx context.Context, id string) error {
	r.mu.Lock()
	defer func() {
		r.joined = false
		r.mu.Unlock()
	}()

	if !r.joined {
		return nil
	}

	// If either of these operations fail it means we can't talk to Redis, effectively booting us from the room when
	// the timeout elapses. We should always try both so that any unused resources will be cleaned up.
	err1 := r.release(ctx, id)
	err2 := r.pubsub.Close()

	if err1 != nil || err2 != nil {
		return ErrRoomGone
	}

	return nil
}

func (r *RedisRoom) Receive(ctx context.Context) ([]byte, error) {
	if !r.joined {
		return nil, nil
	}

	select {
	case <- ctx.Done():
		return nil, ctx.Err()
	case m, ok := <- r.ch:
		if !ok {
			// This check is very important. Since Go will receive on a closed channel forever, if we don't explicitly
			// check if the channel is closed (which will happen when we leave) then any goroutine blocking on this
			// method call will run forever.
			return nil, ErrRoomGone
		}

		return []byte(m.Payload), nil
	}
}

func (r *RedisRoom) Publish(ctx context.Context, message []byte) error {
	if !r.joined {
		return nil
	}

	ch := make(chan error, 1)

	go func() {
		ch <- r.rdb.Publish(ctx, r.key(), message).Err()
		close(ch)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-ch:
		if err != nil {
			return ErrRoomGone
		}
		return nil
	}
}