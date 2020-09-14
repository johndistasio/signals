package main

import (
	"encoding/json"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"io/ioutil"
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
	SessionHandler   *SessionHandler
	SeatHandler      AppHandler
	SignalHandler    AppHandler
	WebsocketHandler AppHandler
}

var pathMatch = regexp.MustCompile(`^/call/([a-zA-Z0-9_\-]+)(/?.*)`)
var pathRewrite = "/call/{call}${2}"

func SplitPath(path string) (string, string, string) {
	segments := strings.Split(strings.TrimPrefix(strings.TrimSuffix(path, "/"), "/"), "/")

	if len(segments) == 2 {
		return segments[0], segments[1], ""
	}

	if len(segments) == 3 {
		return segments[0], segments[1], segments[2]
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

	span.SetTag("http.content_type", r.Header["Content-Type"])

	path, call, op := SplitPath(r.URL.Path)

	var handler AppHandler

	switch {
	case path == "call" && call != "" && op == "":
		handler = s.SeatHandler
		break
	case path == "call" && call != "" && op == "signal":
		handler = s.SignalHandler
		break
	case path == "call" && call != "" && op == "ws":
		handler = s.WebsocketHandler
		break
	default:
		ext.HTTPStatusCode.Set(span, http.StatusNotFound)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	span.SetTag("call.id", call)
	span.SetOperationName(r.Method + " " + pathMatch.ReplaceAllString(r.URL.Path, pathRewrite))

	arw := &AppResponseWriter{w, 0}

	s.SessionHandler.Handle(call, handler).ServeHTTP(arw, r.WithContext(ctx))

	ext.HTTPStatusCode.Set(span, uint16(arw.code))
}

type SeatHandler struct {
	lock Semaphore
}

func (s *SeatHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler.Handle")
		defer span.Finish()

		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		acq, err := s.lock.Acquire(ctx, call, session)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !acq {
			w.WriteHeader(http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

type Signal struct {
	PeerId  string
	CallId  string
	Message string
}

type EndUserMessage struct {
	Error   bool
	Message string
}

type SignalHandler struct {
	lock Semaphore
	pub  Publisher
}

func (s *SignalHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SignalHandler.Handle")
		defer span.Finish()

		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		mime := r.Header["Content-Type"]

		if mime == nil || len(mime) < 0 || mime[0] != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		acq, err := s.lock.Check(ctx, call, session)

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

		var input EndUserMessage

		err = json.Unmarshal(body, &input)

		if err != nil || input.Message == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		signal := Signal{
			PeerId:  session,
			CallId:  call,
			Message: input.Message,
		}

		encoded, err := json.Marshal(signal)

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = s.pub.Publish(ctx, call, encoded)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}
