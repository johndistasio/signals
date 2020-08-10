package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
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

// PubSub declares the methods we need from go-redis's PubSub type so we can mock them for testing.
type PubSub interface {
	Close() error
	ReceiveMessage(ctx context.Context) (*redis.Message, error)
	Subscribe(ctx context.Context, channels ...string) error
}

// RedisRoom is a Room implementation that uses Redis as both a lock manager and a message bus.
type RedisRoom struct{
	name string
	mu sync.Mutex

	rdb  Redis
	pubsub PubSub

	// Flag indicating whether or not we think we're connected to Redis; used to translate errors from the underlying
	// Redis client to "failure because we closed the connection" or "failure because redis is unreachable".
	joined bool
}

func (r *RedisRoom) Name() string {
	return r.key()
}

const roomKeyPrefix = "room:"

const roomSize = 2

func (r *RedisRoom) key() string {
	return roomKeyPrefix + r.name
}

// TODO document arguments
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

// TODO document arguments
var releaseScript = redis.NewScript("return redis.call('ZREM', KEYS[1], ARGV[1])")

func (r *RedisRoom) Enter(ctx context.Context, id string, s int64) (int64, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisRoom.Enter")
	defer span.Finish()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Current timestamp
	now := time.Now()
	nowEpoch := now.Unix()

	// Anything older than this is timed out
	thenEpoch := nowEpoch - s

	acq, err := acquireScript.Run(ctx, r.rdb, []string{r.key()}, thenEpoch, roomSize, nowEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return 0, ErrRoomGone
	}

	if acq != int64(1) {
		err = ErrRoomFull
		ext.LogError(span, err)
		return 0, err
	}

	if r.joined {
		return nowEpoch, nil
	}

	r.pubsub = r.rdb.Subscribe(ctx, r.key())

	r.joined = true

	return nowEpoch, nil
}

func (r *RedisRoom) Leave(ctx context.Context, id string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisRoom.Leave")
	defer span.Finish()

	r.mu.Lock()
	defer func() {
		r.joined = false
		r.mu.Unlock()
	}()

	if !r.joined {
		return ErrRoomLeft
	}

	// If either of these operations fail it means we can't talk to Redis, effectively booting us from the room when
	// the timeout elapses. We should always try both so that any unused resources will be cleaned up.
	err1 := releaseScript.Run(ctx, r.rdb, []string{r.key()}, id).Err()
	err2 := r.pubsub.Close()

	if err1 != nil || err2 != nil {
		return ErrRoomGone
	}

	return nil
}

func (r *RedisRoom) Receive(ctx context.Context) ([]byte, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisRoom.Receive")
	defer span.Finish()

	if !r.joined {
		return nil, ErrRoomLeft
	}

	errChan := make(chan error, 1)
	msgChan := make(chan *redis.Message, 1)

	go func() {
		defer close(errChan)
		defer close(msgChan)

		// returns io.EOF when redis disappears
		m, err := r.pubsub.ReceiveMessage(ctx)

		if err != nil {
			errChan <- err
			return
		}

		msgChan <- m
	}()

	select {
	case <- ctx.Done():
		return nil, ctx.Err()
	case err := <- errChan:
		if !r.joined {
			return nil, ErrRoomLeft
		}
		ext.LogError(span, err)
		return nil, ErrRoomGone
	case m :=<- msgChan:
		return []byte(m.Payload), nil
	}
}

func (r *RedisRoom) Publish(ctx context.Context, message []byte) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisRoom.Publish")
	defer span.Finish()
	if !r.joined {
		return ErrRoomLeft
	}

	ch := make(chan error, 1)

	go func() {
		defer close(ch)
		// returns io.EOF when redis disappears
		ch <- r.rdb.Publish(ctx, r.key(), message).Err()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-ch:
		if err != nil {
			if !r.joined {
				return ErrRoomLeft
			}
			ext.LogError(span, err)
			return ErrRoomGone
		}
		return nil
	}
}