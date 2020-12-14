package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

var ErrPublisher = errors.New("publisher client error")

var ErrPublisherGone = errors.New("publisher backend gone")

type Publisher interface {
	Publish(ctx context.Context, topic string, event interface{}) error
}

const RedisTopicPrefix = "channel:"

type RedisPublisher struct {
	Redis Redis
}

func (r *RedisPublisher) Publish(ctx context.Context, topic string, event interface{}) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisPublisher.Publish")
	defer span.Finish()

	if topic == "" {
		ext.LogError(span, fmt.Errorf("empty topic"))
		return ErrPublisher
	}

	encoded, err := json.Marshal(event)

	if err != nil {
		ext.LogError(span, err)
		return ErrPublisher
	}

	key := RedisTopicPrefix + topic

	err = r.Redis.Publish(ctx, key, encoded).Err()

	if err != nil {
		ext.LogError(span, err)
		return ErrPublisherGone
	}

	return nil
}
