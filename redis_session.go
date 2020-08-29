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

type RedisSessionStore struct {
	redis Redis
}

func (s *RedisSessionStore) Create(ctx context.Context, d time.Duration) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSessionStore.Create")
	defer span.Finish()

	id, err := GenerateSessionId(ctx)

	if err != nil {
		return "", err
	}

	set, err := s.redis.SetNX(ctx, SessionKeyPrefix+id, 1, d).Result()

	if err != nil {
		ext.LogError(span, err)
		return "", ErrSessionBackend
	}

	if !set {
		return "", ErrSessionExists
	}

	return id, nil
}

func (s *RedisSessionStore) Read(ctx context.Context, id string) (time.Duration, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSessionStore.Read")
	defer span.Finish()

	if !ParseSessionId(ctx, id) {
		return 0, ErrSessionInvalid
	}

	ttl, err := s.redis.TTL(ctx, SessionKeyPrefix+id).Result()

	if err != nil {
		ext.LogError(span, err)
		return 0, ErrSessionBackend
	}

	if ttl == -1 {
		return 0, ErrSessionInvalid
	}

	if ttl == -2 {
		return 0, ErrSessionUnknown
	}

	return ttl, nil
}

func (s *RedisSessionStore) Update(ctx context.Context, id string, d time.Duration) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSessionStore.Update")
	defer span.Finish()

	if !ParseSessionId(ctx, id) {
		return ErrSessionInvalid
	}

	set, err := s.redis.Expire(ctx, SessionKeyPrefix+id, d).Result()

	if err != nil {
		ext.LogError(span, err)
		return ErrSessionBackend
	}

	if !set {
		return ErrSessionUnknown
	}

	return nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, id string) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RedisSessionStore.Delete")
	defer span.Finish()

	if !ParseSessionId(ctx, id) {
		return ErrSessionInvalid
	}

	del, err := s.redis.Del(ctx, SessionKeyPrefix+id).Result()

	if err != nil {
		ext.LogError(span, err)
		return ErrSessionBackend
	}

	if del < 1 {
		return ErrSessionUnknown
	}

	return nil
}
