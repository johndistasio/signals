package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"io"
	"net"
	"time"
)

var ErrSemaphore = errors.New("semaphore client error")

var ErrSemaphoreGone = errors.New("semaphore backend gone")

// Semaphore declares a generic interface for a shared locking mechanism.
type Semaphore interface {
	Acquire(ctx context.Context, name string, id string) (bool, error)
	Check(ctx context.Context, name string, id string) (bool, error)
	Release(ctx context.Context, name, id string) error
}

const lockKeyPrefix = "lock:"

// RedisSemaphore implements the counting semaphore from the "Redis in Action" book. Lua scripting is used to avoid
// race conditions between check and set operations. It can and should be shared by multiple goroutines.
type RedisSemaphore struct {
	// Age specifies how long leases are valid for.
	Age time.Duration

	// Count specifies how many active leases are allowed.
	Count int

	// Redis is an implementation of our Redis interface.
	Redis Redis
}

// A Redis Lua script that will clear out expired semaphore holders and then add or renew the lease of the caller
// as appropriate.
//
// Arguments:
// 		1. Lease expiration threshold, as a Unix timestamp. Any lease older than this will be removed.
//		2. Lease count.
//		3. Current Unix timestamp.
//		4. Lease ID.
//
// Returns:
//		0: A lease was not acquired.
//		1: A lease was acquired.
var acquireScript = NewScript(`
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

// A Redis Lua script that will clear out expired semaphore holders and then determine if the caller holds a lease.
//
// Arguments:
// 		1. Lease expiration threshold, as a Unix timestamp. Any lease older than this will be removed.
//		2. Lease ID.
//
// Returns:
//		0: The lease is not held by the given ID.
//		1: Ths lease is held by the given ID.
var checkScript = NewScript(`
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])

    if redis.call('ZSCORE', KEYS[1], ARGV[2]) then
		return 1
	else
		return 0
	end
`)

func (r *RedisSemaphore) Acquire(ctx context.Context, name string, id string) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Acquire")
	defer span.Finish()

	if name == "" || id == "" {
		ext.LogError(span, fmt.Errorf("empty semaphore name or id"))
		return false, ErrSemaphore
	}

	key := lockKeyPrefix + name

	now := time.Now()
	then := now.Add(-r.Age)

	nowEpoch := now.Unix()
	thenEpoch := then.Unix()

	acq, err := acquireScript.Eval(ctx, r.Redis, []string{key}, thenEpoch, r.Count, nowEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return false, translateError(err)
	}

	return acq == int64(1), nil
}

func (r *RedisSemaphore) Check(ctx context.Context, name string, id string) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Check")
	defer span.Finish()

	if name == "" || id == "" {
		ext.LogError(span, fmt.Errorf("empty semaphore name or id"))
		return false, ErrSemaphore
	}

	key := lockKeyPrefix + name

	thenEpoch := time.Now().Add(-r.Age).Unix()

	acq, err := checkScript.Eval(ctx, r.Redis, []string{key}, thenEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return false, translateError(err)
	}

	return acq == int64(1), nil
}

func (r *RedisSemaphore) Release(ctx context.Context, name string, id string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Release")
	defer span.Finish()

	if name == "" || id == "" {
		ext.LogError(span, fmt.Errorf("empty semaphore name or id"))
		return ErrSemaphore
	}

	key := lockKeyPrefix + name

	err := r.Redis.ZRem(ctx, key, id).Err()

	if err != nil {
		ext.LogError(span, err)
		return translateError(err)
	}

	return nil
}

func translateError(err error) error {
	if _, ok := err.(*net.OpError); ok {
		return ErrSemaphoreGone
	}

	if err == io.EOF {
		return ErrSemaphoreGone
	}

	return ErrSemaphore
}
