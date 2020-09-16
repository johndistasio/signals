package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"net/http"
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

		go onWebsocket(wh.pingInterval, wh.readTimeout, conn, pubsub, callId, sessionId)
	})
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func onWebsocket(pingInterval time.Duration, readTimeout time.Duration, conn *websocket.Conn, pubsub *redis.PubSub, callId string, sessionId string) {
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

			onWebsocketSignal(ctx, conn, []byte(msg.Payload), callId, sessionId)
		}
	}
}

func onWebsocketSignal(ctx context.Context, conn *websocket.Conn, message []byte, callId string, sessionId string) {
	span, _ := opentracing.StartSpanFromContext(ctx, "onWebsocketSignal")
	defer span.Finish()

	span.SetTag("call.id", callId)
	span.SetTag("session.id", sessionId)

	var event InternalEvent

	err := json.Unmarshal(message, &event)

	switch {
	case err != nil:
		ext.LogError(span, err)
		return
	case event.CallId != callId:
		ext.LogError(span, errors.Errorf("received unexpected signal for call %s", event.CallId))
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

	err = conn.WriteMessage(websocket.TextMessage, b)

	if err != nil {
		ext.LogError(span, err)
		return
	}
}
