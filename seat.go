package main

import (
	"context"
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
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
		w.WriteHeader(http.StatusNotFound)
		return
	}

	session := sh.Generator(ctx)

	acq, err := sh.Lock.Acquire(ctx, call, session)

	if err != nil {
		ext.LogError(span, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !acq {
		w.WriteHeader(http.StatusConflict)
		return
	}

	event := Event{
		Kind:    MessageKindPeerJoin,
		Call:    call,
		Session: session,
	}

	err = sh.Publisher.Publish(ctx, call, event)

	if err != nil {
		_ = sh.Lock.Release(ctx, call, session)
		ext.LogError(span, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	encoded, _ := json.Marshal(Event{
		Kind:    MessageKindJoin,
		Call:    call,
		Session: session,
	})

	_, _ = w.Write(encoded)
}
