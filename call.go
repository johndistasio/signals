package main

import (
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"net/http"
)

var CallHandler = func(lock Semaphore, pub Publisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "CallHandler")
		defer span.Finish()

		call, seat := ExtractParams(ctx, r)

		if call == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if seat == "" {
			seat = GenerateSessionId(ctx)
		}

		span.SetTag("seat", seat)

		acq, err := lock.Acquire(ctx, call, seat)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		event, _ := json.Marshal(InternalEvent{
			Event: Event{
				Kind: MessageKindPeer,
			},
			PeerId: seat,
			CallId: call,
		})

		err = pub.Publish(ctx, call, event)

		if err != nil {
			_ = lock.Release(ctx, call, seat)
			http.Error(w, "publisher backend unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set(SeatHeader, seat)
		w.WriteHeader(http.StatusNoContent)
	})
}
