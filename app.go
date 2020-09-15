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
	pub  Publisher
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

		event := InternalEvent{
			Event: Event{
				Kind: MessageKindPeer,
			},
			PeerId: session,
			CallId: call,
		}

		bytes, _ := json.Marshal(event)

		err = s.pub.Publish(ctx, call, bytes)

		if err != nil {
			http.Error(w, "publisher backend unavailable", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

const MessageKindPeer = "PEER"

const MessageKindOffer = "OFFER"

const MessageKindAnswer = "ANSWER"

type Event struct {
	Body string `json:"body,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type InternalEvent struct {
	Event
	PeerId string `json:"peerId"`
	CallId string `json:"callId"`
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

		var event Event

		err = json.Unmarshal(body, &event)

		if err != nil || event.Body == "" || (event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		peerMessage := InternalEvent{
			Event:  event,
			PeerId: session,
			CallId: call,
		}

		encoded, err := json.Marshal(peerMessage)

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

type WebsocketHandler struct {
	lock         Semaphore
	redis        Redis
	upgrader     websocket.Upgrader
	readTimeout  time.Duration
	pingInterval time.Duration
}

func (wh *WebsocketHandler) Handle(session string, call string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.Handle")
		defer span.Finish()

		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		held, err := wh.lock.Check(ctx, call, session)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !held {
			w.WriteHeader(http.StatusConflict)
			return
		}

		pubsub := wh.redis.Subscribe(ctx, channelKeyPrefix+call)

		_, err = pubsub.Receive(ctx)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "pubsub backend unavailable", http.StatusInternalServerError)
			return
		}

		// Unwrap the ResponseWriter because AppResponseWriter doesn't implement http.Hijacker.
		conn, err := wh.upgrader.Upgrade(w.(*AppResponseWriter).ResponseWriter, r, nil)

		if err != nil {
			// Need to set this manually because we have to use http.ResponseWriter directly.
			w.(*AppResponseWriter).SetCode(http.StatusBadRequest)
			ext.LogError(span, err)
			return
		}

		go onWebsocket(wh.pingInterval, wh.readTimeout, conn, pubsub, session, call)
	})
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func onWebsocket(pingInterval time.Duration, readTimeout time.Duration, conn *websocket.Conn, pubsub *redis.PubSub, session string, call string) {
	// Initialize a ticket for sending ping messages to the client; we'll use this to detect dead clients.
	ticker := time.NewTicker(pingInterval)

	// Clean up resources upon returning from this function. Any goroutines spawned by this function should tie their
	// lifecycle to the connection or the pubsub so as to avoid leaks.
	defer func() {
		_ = conn.Close()
		_ = pubsub.Close()
		ticker.Stop()
	}()

	// Set up a cancellable context that subroutines can use to signal that we should bail on this websocket.
	ctx, cancel := context.WithCancel(context.Background())

	// Set an initial read deadline. The connection will be closed if we don't receive a pong within this time frame.
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

	// Handle pong messages from the client, resetting the read deadline.
	conn.SetPongHandler(func(appData string) error {
		// Reset the read deadline upon receiving a ping.
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})

	// Handle close messages or disconnects from the client. Duplicates the default close message handler, but calls our
	// cancel function.
	conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		message := websocket.FormatCloseMessage(code, "")
		_ = conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
		return nil
	})

	go func() {
		// Closing the socket is going to mess up writes, so we call our cancel func to bail on the main websocket
		// handler loop. Called in a defer to ensure this happens even in the case of panics.
		defer cancel()

		// Ensure control messages are processed.
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	ch := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second))

			if err != nil {
				return
			}
		case msg, ok := <-ch:
			if !ok {
				// This channel will close when the pubsub is closed, either explicitly by us or due to connection loss.
				return
			}

			onWebsocketSignal(ctx, conn, []byte(msg.Payload), session, call)
		}
	}
}

func onWebsocketSignal(ctx context.Context, conn *websocket.Conn, message []byte, session string, call string) {
	span, _ := opentracing.StartSpanFromContext(ctx, "onWebsocketSignal")
	defer span.Finish()

	span.SetTag("call.id", call)
	span.SetTag("session.id", session)

	var event InternalEvent

	err := json.Unmarshal(message, &event)

	switch {
	case err != nil:
		ext.LogError(span, err)
		return
	case event.CallId != call:
		ext.LogError(span, errors.Errorf("received unexpected signal for call %s", event.CallId))
		return
	case event.PeerId == "":
		ext.LogError(span, errors.New("received signal missing peer ID"))
		return
	case event.Kind != MessageKindPeer && event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer:
		ext.LogError(span, errors.Errorf(`received unexpected message kind "%s" from %s:%s`, event.Kind, event.CallId, event.PeerId))
		return
	case event.PeerId == session:
		return
	}

	span.SetTag("call.peer.id", event.PeerId)

	b, err := json.Marshal(event.Event)

	if err != nil {
		ext.LogError(span, err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, b)

	if err != nil {
		ext.LogError(span, err)
		return
	}
}
