package main

import (
	"context"
	"errors"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

var ErrPublisher = errors.New("publisher client error")

var ErrPublisherGone = errors.New("publisher backend gone")

type Publisher interface {
	Publish(ctx context.Context, channel string, payload []byte) error
}

type RedisPublisher struct {
	Redis Redis
}

func (r *RedisPublisher) Publish(ctx context.Context, channel string, payload []byte) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisPublisher.Publish")
	defer span.Finish()

	if channel == "" || payload == nil || len(payload) == 0 {
		return ErrPublisher
	}

	err := r.Redis.Publish(ctx, channel, payload).Err()

	if err != nil {
		ext.LogError(span, err)
		return ErrPublisherGone
	}

	return nil
}

