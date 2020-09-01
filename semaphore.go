package main

import (
	"context"
	"errors"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"time"
)

var ErrSemaphoreGone = errors.New("semaphore backend gone")

type Semaphore interface {
	Acquire(ctx context.Context, name string, id string) (bool, error)
	Check(ctx context.Context, name string, id string) (bool, error)
	Release(ctx context.Context, name, id string) error
}

type RedisSemaphore struct {
	Age   time.Duration
	Count int
	Redis Redis
}

// TODO document arguments
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

func (r *RedisSemaphore) Acquire(ctx context.Context, name string, id string) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Acquire")
	defer span.Finish()

	now := time.Now()
	then := now.Add(-r.Age)

	nowEpoch := now.Unix()
	thenEpoch := then.Unix()

	acq, err := acquireScript.Eval(ctx, r.Redis, []string{name}, thenEpoch, r.Count, nowEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return false, ErrSemaphoreGone
	}

	return acq == int64(1), nil
}

// TODO document arguments
var checkScript = NewScript(`
	redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])

    if redis.call('ZSCORE', KEYS[1], ARGV[2]) then
		return 1
	else
		return 0
	end
`)

func (r *RedisSemaphore) Check(ctx context.Context, name string, id string) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Check")
	defer span.Finish()

	thenEpoch := time.Now().Add(-r.Age).Unix()

	acq, err := checkScript.Eval(ctx, r.Redis, []string{name}, thenEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return false, ErrSemaphoreGone
	}

	return acq == int64(1), nil
}

func (r *RedisSemaphore) Release(ctx context.Context, name string, id string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Release")
	defer span.Finish()

	err := r.Redis.ZRem(ctx, name, id).Err()

	if err != nil {
		ext.LogError(span, err)
		return ErrSemaphoreGone
	}

	return nil
}
