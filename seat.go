package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
	"time"
)

type SeatHandler struct{}


func (s *SeatHandler) Handler(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()
		sctx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
		span := tracer.StartSpan("SeatHandler.ServeHTTP", ext.RPCServerOption(sctx))
		ctx := opentracing.ContextWithSpan(r.Context(), span)
		defer span.Finish()

		span.SetTag("session.id", session)

		sem := &RedisSemaphore{
			Age:   10 * time.Second,
			Name:  "test",
			Count: 2,
			Redis: NewRedisClient(),
		}

		acq, err := sem.Acquire(ctx, session)

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
