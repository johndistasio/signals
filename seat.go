package main

import (
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
)

type SeatHandler struct {
	lock Semaphore
}

func (s *SeatHandler) Handle(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler.Handle")
		defer span.Finish()

		if r.Method != "GET" {
			ext.HTTPStatusCode.Set(span, http.StatusMethodNotAllowed)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		acq, err := s.lock.Acquire(ctx, "test", session)

		if err != nil {
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

type SignalHandler struct {
	lock Semaphore
}

func (s *SignalHandler) Handle(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, _ := opentracing.StartSpanFromContext(r.Context(), "SignalHandler.Handle")
		defer span.Finish()
		msg := fmt.Sprintf("SignalHandler: %s\n", session)
		_, _ = w.Write([]byte(msg))
	})
}

type WebsocketHandler struct {
	lock Semaphore
}

func (w *WebsocketHandler) Handle(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, _ := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.Handle")
		defer span.Finish()
		msg := fmt.Sprintf("WebsocketHandler: %s\n", session)
		_, _ = w.Write([]byte(msg))
	})
}
