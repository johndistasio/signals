package main

import (
	"context"
	"errors"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/segmentio/ksuid"
)

var ErrSessionBackend = errors.New("session backend gone")

var ErrSessionExists = errors.New("duplicate session ID")

var ErrSessionId = errors.New("failed to generate new session ID")

var ErrSessionInvalid = errors.New("invalid session ID")

var ErrSessionUnknown = errors.New("unknown session ID")

type Session interface {
	ID() string
	Create(ctx context.Context) error
	Renew(ctx context.Context) error
	Expire(ctx context.Context) error
}

func GenerateSessionId(ctx context.Context) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GenerateSessionId")
	defer span.Finish()

	id, err :=  ksuid.NewRandom()

	if err !=  nil {
		ext.LogError(span, err)
		return "", ErrSessionId
	}

	return id.String(), nil
}

func ValidateSessionId(ctx context.Context, id string) bool {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ValidateSessionId")
	defer span.Finish()

	_, err := ksuid.Parse(id)

	if err != nil {
		return false
	}

	return true
}
