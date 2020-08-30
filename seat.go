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

		if r.Method != "GET" {
			ext.HTTPStatusCode.Set(span, http.StatusMethodNotAllowed)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		sem := &RedisSemaphore{
			Age:   10 * time.Second,
			Count: 2,
			Redis: NewRedisClient(),
		}

		acq, err := sem.Acquire(ctx, "test", session)

		if err != nil {
			ext.LogError(span, err)
			//ext.HTTPStatusCode.Set(span, http.StatusInternalServerError)
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			ext.HTTPStatusCode.Set(span, http.StatusConflict)
			w.WriteHeader(http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusOK)

		// https://ndersson.me/post/capturing_status_code_in_net_http/
		ext.HTTPStatusCode.Set(span, http.StatusOK)
	})
}
