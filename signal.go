package main

import (
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"io/ioutil"
	"net/http"
)

type SignalHandler struct {
	Lock      Semaphore
	Publisher Publisher
	MaxRead   int64
}

func (s *SignalHandler) Handle(callId string, sessionId string) http.Handler {
	readLimit := s.MaxRead

	if readLimit == 0 {
		readLimit = 512
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SignalHandler")
		defer span.Finish()

		acq, err := s.Lock.Check(ctx, callId, sessionId)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))

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

		// TODO the client should provide this
		event.Call = callId

		peerMessage := Event{
			Call:    event.Call,
			Session: sessionId, // TODO client should provide this, get rid of header
			Kind:    event.Kind,
			Body:    event.Body,
		}

		encoded, err := json.Marshal(peerMessage)

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = s.Publisher.Publish(ctx, callId, encoded)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}
