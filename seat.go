package main

import (
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"net/http"
)

type SeatHandler struct {
	Lock Semaphore
	Pub  Publisher
}

func (s *SeatHandler) Handle(callId string, sessionId string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler")
		defer span.Finish()

		acq, err := s.Lock.Acquire(ctx, callId, sessionId)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		event, _ := json.Marshal(Event{
			Kind:    MessageKindPeerJoin,
			Call:    callId,
			Session: sessionId,
		})

		err = s.Pub.Publish(ctx, callId, event)

		if err != nil {
			_ = s.Lock.Release(ctx, callId, sessionId)
			http.Error(w, "publisher backend unavailable", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
