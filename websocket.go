package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/pkg/errors"
	"net"
	"net/http"
	"time"
)

type WebsocketHandler struct {
	lock        Semaphore
	redis       Redis
	upgrader    websocket.Upgrader
	readTimeout time.Duration
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

		go onWebsocket(wh.readTimeout, conn, pubsub, session, call)
	})
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func onWebsocket(timeout time.Duration, conn *websocket.Conn, pubsub *redis.PubSub, session string, call string) {
	// Clean up IO resources upon returning from this function. Any goroutines spawned by this function should tie their
	// lifecycle to the connection or the pubsub so as to avoid leaks.
	defer func() {
		_ = conn.Close()
		_ = pubsub.Close()
	}()

	// Set up a cancellable context that subroutines can use to signal that we should bail on this websocket.
	ctx, cancel := context.WithCancel(context.Background())

	// Set an initial read deadline. The connection will be closed if we don't receive a ping within this time frame.
	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	// TODO manage read deadline with pong instead

	// Handle ping messages from the client. Duplicates the default ping handler, but also resets the read deadline on
	// each ping since we aren't expecting any application-level input from the client, and calls our cancel function
	// on unrecoverable connection errors.
	conn.SetPingHandler(func(appData string) error {
		// Reset the read deadline upon receiving a ping.
		_ = conn.SetReadDeadline(time.Now().Add(timeout))

		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(1*time.Second))

		if err == websocket.ErrCloseSent {
			cancel()
			return nil
		}

		if e, ok := err.(net.Error); ok && !e.Temporary() {
			cancel()
			return nil
		}

		if err != nil {
			cancel()
			return err
		}

		return nil
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
		// A closure of the socket is going to mess up writes, so we call our cancel func to bail on the main websocket
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

	var signal Signal

	err := json.Unmarshal(message, &signal)

	if err != nil {
		ext.LogError(span, err)
		span.Finish()
		return
	}

	span.SetTag("call.peer.id", signal.PeerId)

	if signal.CallId != call {
		ext.LogError(span, errors.Errorf("received unexpected signal for call %s", signal.CallId))
		return
	}

	if signal.PeerId == session {
		return
	}

	m := EndUserMessage{
		Message: signal.Message,
	}

	b, err := json.Marshal(m)

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
