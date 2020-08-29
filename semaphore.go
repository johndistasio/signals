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
	Acquire(ctx context.Context, id string) (bool, error)
	Release(ctx context.Context, id string) error
}

type RedisSemaphore struct {
	Age   time.Duration
	Count int
	Name string
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

func (r *RedisSemaphore) Acquire(ctx context.Context, id string) (bool, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Acquire")
	defer span.Finish()

	now := time.Now()
	then := now.Add(-r.Age)

	nowEpoch := now.Unix()
	thenEpoch := then.Unix()

	acq, err := acquireScript.Eval(ctx, r.Redis, []string{r.Name}, thenEpoch, r.Count, nowEpoch, id).Result()

	if err != nil {
		ext.LogError(span, err)
		return false, ErrSemaphoreGone
	}

	return acq == int64(1), nil
}

func (r *RedisSemaphore) Release(ctx context.Context, id string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSemaphore.Release")
	defer span.Finish()

	err := r.Redis.ZRem(ctx, r.Name, id).Err()

	if err != nil {
		ext.LogError(span, err)
		return ErrSemaphoreGone
	}

	return nil
}
