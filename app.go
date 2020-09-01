package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
	"regexp"
	"strings"
)

type AppHandler interface {
	Handle(session string, call string) http.Handler
}

// AppResponseWriter wraps the http.ResponseWriter type and stores a response code so it can be added to a trace
// after child handlers have processed the request.
type AppResponseWriter struct {
	http.ResponseWriter
	code int
}

// SetCode is an escape hatch for handlers that need to unwrap the underlying http.ResponseWriter for some reason.
func (w *AppResponseWriter) SetCode(code int) {
	w.code = code
}

// Write implements http.ResponseWriter.
func (w *AppResponseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

// WriterHeader implements http.ResponseWriter and stores the HTTP response code.
func (w *AppResponseWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// Header implements http.ResponseWriter.
func (w *AppResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}


type App struct {
	SessionMiddleware *SessionHandler
	SeatHandler       AppHandler
	SignalHandler     AppHandler
	WebsocketHandler  AppHandler
}

var pathMatch = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)(/?.*)`)
var pathRewrite = "/call/{call}${2}"

func SplitPath(path string) (string, string, string) {
	matches := strings.Split(strings.TrimPrefix(strings.TrimSuffix(path, "/"), "/"), "/")

	if len(matches ) == 2 {
		return matches[0], matches[1], ""
	}

	if len(matches ) == 3 {
		return matches[0], matches[1], matches[2]
	}

	return "", "", ""
}

func (s *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracer := opentracing.GlobalTracer()
	spanCtx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
	span := tracer.StartSpan("App.ServeHTTP", ext.RPCServerOption(spanCtx))
	ctx := opentracing.ContextWithSpan(r.Context(), span)
	defer span.Finish()

	ext.HTTPUrl.Set(span, r.URL.String())
	ext.HTTPMethod.Set(span, r.Method)
	ext.PeerAddress.Set(span, r.RemoteAddr)

	head, call, op := SplitPath(r.URL.Path)

	var h AppHandler

	switch {
	case head == "call" && call != "" && op == "":
		h = s.SeatHandler
		break
	case head == "call" && call != "" && op == "signal":
		h = s.SignalHandler
		break
	case head == "call" && call != "" && op == "ws":
		h = s.WebsocketHandler
		break
	default:
		ext.HTTPStatusCode.Set(span, http.StatusNotFound)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	span.SetTag("call.id", call)
	span.SetOperationName(pathMatch.ReplaceAllString(r.URL.Path, pathRewrite))

	arw := &AppResponseWriter{w, 0}

	s.SessionMiddleware.Handle(call, h).ServeHTTP(arw, r.WithContext(ctx))

	if arw.code != 0 {
		ext.HTTPStatusCode.Set(span, uint16(arw.code))
	}
}
