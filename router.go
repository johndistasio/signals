package main

import (
	"net/http"
	"regexp"
)

var seatRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/?}$a`)

var signalRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/signal/?$`)

var wsRegex = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)/ws/?$`)

var RouterHandler = http.HandlerFunc(Router)

func Router(w http.ResponseWriter, r *http.Request) {

	switch {
	case seatRegex.MatchString(r.URL.Path):
		break
	case signalRegex.MatchString(r.URL.Path):
		break
	case wsRegex.MatchString(r.URL.Path):
		break
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

