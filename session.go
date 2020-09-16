package main

import (
	"context"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/segmentio/ksuid"
	"net/http"
	"strings"
)

func GenerateSessionId(ctx context.Context) string {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GenerateSessionId")
	defer span.Finish()

	id, err := ksuid.NewRandom()

	if err != nil {
		ext.LogError(span, err)
		return ""
	}

	return id.String()
}

func ParseSessionId(ctx context.Context, id string) bool {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ParseSessionId")
	defer span.Finish()

	_, err := ksuid.Parse(id)

	if err != nil {
		return false
	}

	return true
}

func ExtractCallId(ctx context.Context, r *http.Request) string {
	segments := strings.Split(strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/"), "/")

	if len(segments) >= 2 {
		return segments[1]
	}

	return ""
}

type SessionParameterHandler interface {
	Handle(callId string, sessionId string) http.Handler
}

type CreateSessionFunc func(context.Context) string

type ValidateSessionFunc func(context.Context, string) bool

type ExtractCallFunc func(context.Context, *http.Request) string

// SessionHandler is HTTP middleware that manages session cookies on incoming requests.
type SessionHandler struct {
	// CreateSessionId is a function that generates a new session ID.
	CreateSessionId CreateSessionFunc

	// ValidateSessionId is a function that determines if a given session ID is valid.
	ValidateSessionId ValidateSessionFunc

	// TODO document me
	ExtractCallId ExtractCallFunc
}

func (s *SessionHandler) Handle(next SessionParameterHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SessionHandler.Handle")
		defer span.Finish()

		sessionId := r.Header.Get(SessionHeader)

		if sessionId == "" {
			sessionId = s.CreateSessionId(ctx)
		} else {
			if !s.ValidateSessionId(ctx, sessionId) {
				sessionId = s.CreateSessionId(ctx)
			}
		}

		w.Header().Set(SessionHeader, sessionId)
		span.SetTag("session.id", sessionId)

		callId := s.ExtractCallId(ctx, r)

		if callId == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if next != nil {
			next.Handle(callId, sessionId).ServeHTTP(w, r.WithContext(ctx))
		}
	})
}
