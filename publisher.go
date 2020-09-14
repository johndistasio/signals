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
	Publish(ctx context.Context, topic string, payload []byte) error
}

const channelKeyPrefix = "channel:"

type RedisPublisher struct {
	Redis Redis
}

func (r *RedisPublisher) Publish(ctx context.Context, topic string, payload []byte) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisPublisher.Publish")
	defer span.Finish()

	if topic == "" || payload == nil || len(payload) == 0 {
		return ErrPublisher
	}

	key := channelKeyPrefix + topic

	err := r.Redis.Publish(ctx, key, payload).Err()

	if err != nil {
		ext.LogError(span, err)
		return ErrPublisherGone
	}

	return nil
}

