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

func (s *SignalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span, ctx := opentracing.StartSpanFromContext(r.Context(), "SignalHandler")
	defer span.Finish()

	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, s.MaxRead))

	if err != nil {
		ext.LogError(span, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(body) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var event Event

	err = json.Unmarshal(body, &event)

	if  err != nil ||
		event.Body == "" || event.Call == "" || event.Session == "" ||
		(event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	acq, err := s.Lock.Check(ctx, event.Call, event.Session)

	if err != nil {
		ext.LogError(span, err)
		http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
		return
	}

	if !acq {
		w.WriteHeader(http.StatusConflict)
		return
	}

	err = s.Publisher.Publish(ctx, event.Call, body)

	if err != nil {
		ext.LogError(span, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
