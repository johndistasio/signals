package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
	"time"
)

func SeatHandler(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler")
		defer span.Finish()

		span.SetTag("session.id", session)

		sem := &RedisSemaphore{
			Age:   10 * time.Second,
			Count: 2,
			Redis: NewRedisClient(),
		}

		acq, err := sem.Acquire(ctx, "test", session)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}
