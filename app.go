package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
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

		err = s.redis.Publish(ctx, ChannelKeyPrefix+call, encoded).Err()

		if err != nil {
			ext.LogError(span, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

type WebsocketHandler struct {
	call     string
	session  string
	pubsub   *redis.PubSub
	conn     *websocket.Conn
	lock     Semaphore
	redis    Redis
	stopChan chan int
}

var upgrader = websocket.Upgrader{}

func (wh *WebsocketHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.Handle")
		defer span.Finish()

		wh.stopChan = make(chan int)
		wh.session = session
		wh.call = call

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
		wh.conn = conn

		if err != nil {
			// Need to set this manually because we have to use http.ResponseWriter directly.
			w.(*AppResponseWriter).SetCode(http.StatusBadRequest)
			ext.LogError(span, err)
			return
		}

		conn.SetCloseHandler(func(code int, text string) error {
			close(wh.stopChan)
			message := websocket.FormatCloseMessage(code, "")
			_ = conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
			return nil
		})

		pubsub := wh.redis.Subscribe(ctx, ChannelKeyPrefix+call)
		_, err = pubsub.Receive(ctx)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "pubsub backend unavailable", http.StatusInternalServerError)
			return
		}

		wh.pubsub = pubsub

		// Ensure control messages are processed.
		go func() {
			for {
				if _, _, err := conn.NextReader(); err != nil {
					_ = conn.Close()
					return
				}
			}
		}()

		go wh.loop()
	})
}

func (wh *WebsocketHandler) onMessage() {

}

func (wh *WebsocketHandler) loop() {
	defer func() {
		_ = wh.conn.Close()
		_ = wh.pubsub.Close()
	}()

	ch := wh.pubsub.Channel()

	for {
		select {
		case _, closed := <-wh.stopChan:
			if !closed {
				return
			}
		case remote, ok := <-ch:
			span, _ := opentracing.StartSpanFromContext(context.Background(), "WebsocketHandler.loop")
			span.SetTag("call.id", wh.call)
			span.SetTag("session.id", wh.session)

			if !ok {
				ext.LogError(span, errors.New("pubsub backend gone"))
				span.Finish()
				return
			}

			var signal Signal

			err := json.Unmarshal([]byte(remote.Payload), &signal)

			if err != nil {
				ext.LogError(span, err)
				span.Finish()
				return
			}

			span.SetTag("call.peer.id", signal.PeerId)

			if signal.CallId != wh.call {
				ext.LogError(span, errors.Errorf("received unexpected signal for call %s", signal.CallId))
				span.Finish()
				return
			}

			if signal.PeerId == wh.session {
				span.Finish()
				continue
			}

			m := EndUserMessage{
				Message: signal.Message,
			}

			b, err := json.Marshal(m)

			if err != nil {
				ext.LogError(span, err)
				span.Finish()
				return
			}

			err = wh.conn.WriteMessage(websocket.TextMessage, b)

			if err != nil {
				ext.LogError(span, err)
				span.Finish()
				return
			}

		}
	}
}
