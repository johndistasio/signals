package main

import (
	"context"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"time"
)

// RedisSession is a Session stored in Redis. Sessions are stored as keys and use Redis' TTL functionality to age out.
// Key values are the Unix timestamp of session creation.
type RedisSession struct {
	r  Redis
	d  time.Duration
	id string
}

const SessionKeyPrefix = "session:"

func NewRedisSession(r Redis, d time.Duration, id string) *RedisSession {
	return &RedisSession{r, d, id}
}

func (s *RedisSession) ID() string {
	return s.id
}

func (s *RedisSession) Create(ctx context.Context) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSession.Create")
	defer span.Finish()

	val := time.Now().Unix()

	set, err := s.r.SetNX(ctx, SessionKeyPrefix+s.id, val, s.d).Result()

	if err != nil {
		ext.LogError(span, err)
		return ErrSessionBackend
	}

	if !set {
		return ErrSessionExists
	}

	return nil
}

func (s *RedisSession) Renew(ctx context.Context) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSession.Renew")
	defer span.Finish()

	set, err := s.r.Expire(ctx, SessionKeyPrefix+s.id, s.d).Result()

	if err != nil {
		ext.LogError(span, err)
		return ErrSessionBackend
	}

	if !set {
		return ErrSessionUnknown
	}

	return nil
}

func (s *RedisSession) Expire(ctx context.Context) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSession.Expire")
	defer span.Finish()

	del, err := s.r.Del(ctx, SessionKeyPrefix+s.id).Result()

	if err != nil {
		ext.LogError(span, err)
		return ErrSessionBackend
	}

	if del < 1 {
		return ErrSessionUnknown
	}

	return nil
}

