package main

import (
	"github.com/opentracing/opentracing-go"
	"net/http"
	"regexp"
)

var seatRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/?$`)

var signalRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/signal/?$`)

var wsRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/ws/?$`)

type AppHandler interface {
	Handle(string) http.Handler
}

type RoutingMiddleware struct {
	SessionMiddleware *SessionMiddleware
	SeatHandler       AppHandler
	SignalHandler     AppHandler
	WebsocketHandler  AppHandler
}

func (s *RoutingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span, ctx := opentracing.StartSpanFromContext(r.Context(), "RoutingMiddleware.Handle")
	defer span.Finish()

	// TODO add the value of the call ID path element as a span tag here

	var h AppHandler

	switch {
	case seatRegex.MatchString(r.URL.Path):
		h = s.SeatHandler
		break
	case signalRegex.MatchString(r.URL.Path):
		h = s.SignalHandler
		break
	case wsRegex.MatchString(r.URL.Path):
		h = s.WebsocketHandler
		break
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	s.SessionMiddleware.Handle(h).ServeHTTP(w, r.WithContext(ctx))
}
