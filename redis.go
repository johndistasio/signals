package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"sync"
)

// Redis declares the methods we need from go-redis so that we can mock them for testing.
type Redis interface {
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
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

func (r *RedisRoom) key() string {
	return roomKeyPrefix + r.name
}

func (r *RedisRoom) Join(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.acquire(ctx); err != nil {
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

// TODO implement me
func (r *RedisRoom) acquire(ctx context.Context) error {
	acquired := true

	// TODO need to make distinction between room full and room gone

	if !acquired {
		return ErrRoomFull
	}

	return nil
}

func (r *RedisRoom) Leave(ctx context.Context) error {
	r.mu.Lock()
	defer func() {
		r.joined = false
		r.mu.Unlock()
	}()

	if !r.joined {
		return nil
	}

	// If either of these operations fail it means we can't talk to Redis, effectively booting us from the room.
	// We should always try both so that any unused resources will be cleaned up.
	err1 := r.release(ctx)
	err2 := r.pubsub.Close()

	if err1 != nil || err2 != nil {
		return ErrRoomGone
	}

	return nil
}

// TODO implement me
func (r *RedisRoom) release(ctx context.Context) error {
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