package main

import (
	"fmt"
	"github.com/gorilla/websocket"
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

var upgrader = websocket.Upgrader{}

func (w *WebsocketHandler) Handle(session string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, _ := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.Handle")
		defer span.Finish()

		// Unwrap the ResponseWriter because TracingResponseWriter doesn't implement http.Hijacker.
		conn, err := upgrader.Upgrade(w.(*TracingResponseWriter).ResponseWriter, r, nil)

		if err != nil {
			ext.LogError(span, err)
			return
		}

		go func() {
			defer conn.Close()
			for {
				_, msg, err := conn.ReadMessage()

				if err != nil {
					return
				}

				err = conn.WriteMessage(websocket.TextMessage, msg)

				if err != nil {
					return
				}
			}
		}()
	})
}
