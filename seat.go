package main

import (
	"context"
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"net/http"
)

type SeatHandler struct {
	Generator func(context.Context) string
	Lock      Semaphore
	Publisher Publisher
}

// Attempts to reserve a seat for the client in the call specified by the path. On success, emits a "new peer" event
// for other clients in the room and returns a token that the client can use to send and receive signals from other
// clients. On failure, returns 404 when no call is specified, 409 when no seats are available on the call, and
// 500 otherwise.
func (sh *SeatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler.ServeHTTP")
	defer span.Finish()

	call := ExtractCallId(ctx, r)

	if call == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	session := sh.Generator(ctx)

	acq, err := sh.Lock.Acquire(ctx, call, session)

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
		Call:    call,
		Session: session,
	})

	err = sh.Publisher.Publish(ctx, call, event)

	if err != nil {
		_ = sh.Lock.Release(ctx, call, session)
		http.Error(w, "publisher backend unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	event, _ = json.Marshal(Event{
		Kind:    MessageKindJoin,
		Call:    call,
		Session: session,
	})

	_, _ = w.Write(event)
}
