package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"log"
	"net/http"
	"sync"
	"time"
)

type WebsocketHandler struct {
	lock         Semaphore
	redis        Redis
	upgrader     websocket.Upgrader
	readTimeout  time.Duration
	pingInterval time.Duration
}

func (wh *WebsocketHandler) Handle(callId string, sessionId string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span, ctx := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler")
		defer span.Finish()

		held, err := wh.lock.Check(ctx, callId, sessionId)

		if err != nil {
			http.Error(w, "seat backend unavailable", http.StatusInternalServerError)
			return
		}

		if !held {
			w.WriteHeader(http.StatusConflict)
			return
		}

		pubsub := wh.redis.Subscribe(ctx, channelKeyPrefix+callId)

		_, err = pubsub.Receive(ctx)

		if err != nil {
			ext.LogError(span, err)
			http.Error(w, "pubsub backend unavailable", http.StatusInternalServerError)
			return
		}

		// Unwrap the ResponseWriter because TracingResponseWriter doesn't implement http.Hijacker.
		conn, err := wh.upgrader.Upgrade(w.(*TracingResponseWriter).ResponseWriter, r, nil)

		if err != nil {
			// Need to set this manually because we have to use http.ResponseWriter directly.
			w.(*TracingResponseWriter).SetCode(http.StatusBadRequest)
			ext.LogError(span, err)
			return
		}

		ws := &WebsocketSession{
			readTimeout:  wh.readTimeout,
			pingInterval: wh.pingInterval,
			conn:         conn,
			pubsub:       pubsub,
			callId:       callId,
			mu:           new(sync.Mutex),
		}

		go ws.onWebsocket(sessionId)
	})
}

type WebsocketSession struct {
	readTimeout  time.Duration
	pingInterval time.Duration
	conn         *websocket.Conn
	pubsub       *redis.PubSub
	callId       string
	mu           *sync.Mutex
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func (ws *WebsocketSession) onWebsocket(sessionId string) {
	// Initialize a ticket for sending ping messages to the client; we'll use this to detect dead clients.
	ticker := time.NewTicker(ws.pingInterval)

	// Clean up resources upon returning from this function. Any goroutines spawned by this function should tie their
	// lifecycle to the connection or the pubsub so as to avoid leaks.
	defer func() {
		_ = ws.conn.Close()
		_ = ws.pubsub.Close()
		ticker.Stop()
	}()

	// Set up a cancellable context that subroutines can use to signal that we should bail on this websocket.
	ctx, cancel := context.WithCancel(context.Background())

	// Set an initial read deadline. The connection will be closed if we don't receive a pong within this time frame.
	_ = ws.conn.SetReadDeadline(time.Now().Add(ws.readTimeout * 2))

	// Handle pong messages from the client, resetting the read deadline.
	ws.conn.SetPongHandler(func(appData string) error {
		// Reset the read deadline upon receiving a ping.
		return ws.conn.SetReadDeadline(time.Now().Add(ws.readTimeout))
	})

	// Handle close messages or disconnects from the client. Duplicates the default close message handler, but calls our
	// cancel function.
	ws.conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		message := websocket.FormatCloseMessage(code, "")
		_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
		return nil
	})

	clientCh := make(chan Event)

	go func() {
		// Closing the socket is going to mess up writes, so we call our cancel func to bail on the main websocket
		// handler loop. Called in a defer to ensure this happens even in the case of panics.
		defer cancel()
		defer close(clientCh)

		// Ensure control messages are processed.
		for {
			/*
				if _, _, err := ws.conn.NextReader(); err != nil {
					return
				}
			*/

			_, message, err := ws.conn.ReadMessage()

			if err != nil {
				return
			}

			err = ws.onClientSignal(ctx, message, sessionId, clientCh)

			if err != nil {
				return
			}
		}
	}()

	ch := ws.pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := ws.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second))

			if err != nil {
				return
			}
		case msg, ok := <-ch:
			if !ok {
				// This channel will close when the pubsub is closed, either explicitly by us or due to connection loss.
				return
			}

			ws.onPeerSignal(ctx, []byte(msg.Payload), sessionId)
		case msg, ok := <-clientCh:
			if !ok {
				// This channel will close when the reader experiences an error.
				return
			}

			log.Printf("received %v\n", msg)

		}
	}
}

func (ws *WebsocketSession) onClientSignal(ctx context.Context, message []byte, sessionId string, ch chan Event) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.onClientSignal")
	defer span.Finish()

	span.SetTag("session.id", sessionId)

	var event Event

	err := json.Unmarshal(message, &event)

	if err != nil {
		ext.LogError(span, err)
		return err
	}

	span.SetTag("event.kind", event.Kind)
	span.SetTag("call.id", event.Call)

	switch {
	case event.Kind != MessageKindJoin:
		err = errors.Errorf(`received unexpected event kind "%s"`, event.Kind)
	case event.Call == "":
		err = errors.Errorf("received event with empty call id")
	case event.Session != sessionId:
		err = errors.Errorf(`received unexpected session id "%s"`, event.Session)
	default:
		// TODO validate the event
		ch <- event
		return nil
	}

	ext.LogError(span, err)
	return err
}

func (ws *WebsocketSession) onPeerSignal(ctx context.Context, message []byte, sessionId string) {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.onPeerSignal")
	defer span.Finish()

	span.SetTag("call.id", ws.callId)
	span.SetTag("session.id", sessionId)

	// TODO debug
	log.Printf("onPeerSignal: %s\n", string(message))

	var event InternalEvent

	err := json.Unmarshal(message, &event)

	switch {
	case err != nil:
		ext.LogError(span, err)
		return
	case event.CallId != ws.callId:
		ext.LogError(span, errors.Errorf(`received unexpected signal for call "%s"`, event.CallId))
		return
	case event.PeerId == "":
		ext.LogError(span, errors.New("received signal missing peer ID"))
		return
	case event.Kind != MessageKindPeer && event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer:
		ext.LogError(span, errors.Errorf(`received unexpected message kind "%s" from %s:%s`, event.Kind, event.CallId, event.PeerId))
		return
	case event.PeerId == sessionId:
		return
	}

	span.SetTag("call.peer.id", event.PeerId)

	b, err := json.Marshal(event.Event)

	if err != nil {
		ext.LogError(span, err)
		return
	}

	err = ws.conn.WriteMessage(websocket.TextMessage, b)

	if err != nil {
		ext.LogError(span, err)
		return
	}
}
