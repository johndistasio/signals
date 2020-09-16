package main

import (
	"context"
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"io/ioutil"
	"net/http"
)

func ExtractParams(ctx context.Context, r *http.Request) (string, string) {
	_, call, _ := SplitPath(r.URL.Path)

	seat := r.Header.Get("Seat")

	if !ParseSessionId(ctx, seat) {
		seat = ""
	}

	return call, seat
}

var SignalHandler2 = func(lock Semaphore, pub Publisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SignalHandler")
		defer span.Finish()

		call, seat := ExtractParams(ctx, r)

		if call == "" || seat == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		acq, err := lock.Check(ctx, call, seat)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		// TODO make limit configurable
		body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, 512))

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if len(body) < 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var event Event

		err = json.Unmarshal(body, &event)

		if err != nil || event.Body == "" || (event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		peerMessage := InternalEvent{
			Event:  event,
			PeerId: seat,
			CallId: call,
		}

		encoded, err := json.Marshal(peerMessage)

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = pub.Publish(ctx, call, encoded)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

