package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"
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

	if len(matches) == 2 {
		return matches[0], matches[1], ""
	}

	if len(matches) == 3 {
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

type Signal struct {
	PeerId  string
	Call    string
	Message string
}

type EndUserMessage struct {
	Error   bool
	Message string
}

type SeatHandler struct {
	lock Semaphore
}

const CallKeyPrefix = "call:"

func (s *SeatHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "SeatHandler.Handle")
		defer span.Finish()

		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		acq, err := s.lock.Acquire(ctx, CallKeyPrefix+call, session)

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

type SignalHandler struct {
	lock  Semaphore
	redis Redis
}

const ChannelKeyPrefix = "channel:"

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

		acq, err := s.lock.Check(ctx, CallKeyPrefix+call, session)

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

		signal := Signal{
			PeerId:  session,
			Call:    call,
			Message: string(body),
		}

		msg, err := json.Marshal(signal)

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = s.redis.Publish(ctx, ChannelKeyPrefix+call, msg).Err()

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

type WebsocketHandler struct {
	lock  Semaphore
	redis Redis
	conn  *websocket.Conn
}

var upgrader = websocket.Upgrader{}

func (wh *WebsocketHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.Handle")
		defer span.Finish()

		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		held, err := wh.lock.Check(ctx, CallKeyPrefix+call, session)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !held {
			w.WriteHeader(http.StatusConflict)
			return
		}

		// Unwrap the ResponseWriter because AppResponseWriter doesn't implement http.Hijacker.
		conn, err := upgrader.Upgrade(w.(*AppResponseWriter).ResponseWriter, r, nil)
		defer conn.Close()

		wh.conn = conn

		if err != nil {
			// Need to set this manually because we have to use http.ResponseWriter directly.
			w.(*AppResponseWriter).SetCode(http.StatusBadRequest)
			ext.LogError(span, err)
			return
		}

		pubsub := wh.redis.Subscribe(ctx, ChannelKeyPrefix+call)
		defer pubsub.Close()
		ch := pubsub.Channel()

		// TODO make this configurable
		checker := time.NewTicker(30 * time.Second)
		defer checker.Stop()

		for {
			select {
			case <-checker.C:
				held, err := wh.lock.Check(ctx, CallKeyPrefix+call, session)

				if err != nil || !held {
					// TODO return an error to the client
					return
				}
			case remote, ok := <-ch:
				if !ok {
					// TODO return an error to the client
					ext.LogError(span, errors.New("pubsub backend gone"))
					return
				}

				var signal Signal

				err := json.Unmarshal([]byte(remote.Payload), &signal)

				if err != nil {
					ext.LogError(span, err)
					return
				}

				if signal.Call != call {
					ext.LogError(span, errors.Errorf("received unexpected signal for call %s", signal.Call))
					return
				}

				if signal.PeerId == session {
					continue
				}

				msg := EndUserMessage{
					Message: signal.Message,
				}

				err = wh.writeToSocket(ctx, msg)

				if err != nil {
					return
				}
			}
		}
	})
}

func (wh *WebsocketHandler) writeToSocket(ctx context.Context, m EndUserMessage) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "WebsocketHandler.writeToSocket")
	defer span.Finish()

	b, err := json.Marshal(m)

	if err != nil {
		return err
	}

	err = wh.conn.WriteMessage(websocket.TextMessage, b)

	if err != nil {
		return err
	}

	return nil
}
