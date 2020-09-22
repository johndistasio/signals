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
	joinTimeout  time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration
}

func (wh *WebsocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span, _ := opentracing.StartSpanFromContext(r.Context(), "WebsocketHandler.ServeHTTP")
	defer span.Finish()

	// Unwrap the ResponseWriter because TracingResponseWriter doesn't implement http.Hijacker.
	conn, err := wh.upgrader.Upgrade(w.(*TracingResponseWriter).ResponseWriter, r, nil)

	if err != nil {
		// Need to set this manually because we have to use http.ResponseWriter directly.
		w.(*TracingResponseWriter).SetCode(http.StatusBadRequest)
		ext.LogError(span, err)
		return
	}

	ws := &WebsocketSession{
		lock:         wh.lock,
		redis:        wh.redis,
		joinTimeout: wh.joinTimeout,
		readTimeout:  wh.readTimeout,
		pingInterval: wh.pingInterval,
		conn:         conn,
	}

	go ws.onWebsocket()
}

type WebsocketSession struct {
	lock         Semaphore
	redis        Redis
	joinTimeout  time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration
	conn         *websocket.Conn
	pubsub       *redis.PubSub
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func (ws *WebsocketSession) onWebsocket() {
	// Initialize a ticket for sending ping messages to the client; we'll use this to detect dead clients.
	ticker := time.NewTicker(ws.pingInterval)

	// Clean up resources upon returning from this function. Any goroutines spawned by this function should tie their
	// lifecycle to the connection or the pubsub so as to avoid leaks.
	defer func() {
		_ = ws.conn.Close()
		ticker.Stop()
	}()

	// Set up a cancellable context that subroutines can use to signal that we should bail on this websocket.
	ctx, cancel := context.WithCancel(context.Background())

	// Set an initial read deadline. The connection will be closed if we don't receive a pong within this time frame.
	if ws.joinTimeout > 0 {
		_ = ws.conn.SetReadDeadline(time.Now().Add(ws.joinTimeout))
	}

	// Handle close messages or disconnects from the client. Duplicates the default close message handler, but calls our
	// cancel function.
	ws.conn.SetCloseHandler(func(code int, text string) error {
		cancel()
		message := websocket.FormatCloseMessage(code, text)
		_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
		return nil
	})

	// Wait for "join call" handshake message
	_, message, err := ws.conn.ReadMessage()

	if err != nil {
		return
	}

	join, err := ws.onClientSignal(ctx, message)

	if err != nil {
		message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bad handshake")
		_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
		return
	}

	// Check join status
	if held, err := ws.lock.Check(ctx, join.Call, join.Session); err != nil || !held {
		message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bad seat")
		_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
		return
	}

	// TODO write confirmation?

	// Handle pong messages from the client, resetting the read deadline.
	ws.conn.SetPongHandler(func(appData string) error {
		span, ctx := opentracing.StartSpanFromContext(ctx, "WebsocketSession.pongHandler")
		defer span.Finish()

		span.SetTag("session", join.Session)
		span.SetTag("call", join.Call)

		// Reset the read deadline upon receiving a ping.
		if ws.readTimeout > 0 {
			_ = ws.conn.SetReadDeadline(time.Now().Add(ws.readTimeout))
		}

		if held, err := ws.lock.Acquire(ctx, join.Call, join.Session); err != nil || !held {
			message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "seat renewal failed")
			_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
			return err
		}

		return nil
	})


	// Subscribe

	pubsub := ws.redis.Subscribe(ctx, channelKeyPrefix+join.Call)
	defer pubsub.Close()

	_, err = pubsub.Receive(ctx)

	if err != nil {
		return
	}

	go func() {
		// Closing the socket is going to mess up writes, so we call our cancel func to bail on the main websocket
		// handler loop. Called in a defer to ensure this happens even in the case of panics.
		defer cancel()

		// Ensure control messages are processed.
		for {
			if _, _, err := ws.conn.NextReader(); err != nil {
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
			err := ws.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second))

			if err != nil {
				return
			}
		case msg, ok := <-ch:
			if !ok {
				// This channel will close when the pubsub is closed, either explicitly by us or due to connection loss.
				return
			}

			if held, err := ws.lock.Check(ctx, join.Call, join.Session); err != nil || !held {
				return
			}

			event, err := ws.onPeerSignal(ctx, []byte(msg.Payload), join.Call, join.Session)

			b, err := json.Marshal(event)

			if err != nil {
				return
			}

			err = ws.conn.WriteMessage(websocket.TextMessage, b)

			if err != nil {
				return
			}
		}
	}
}

func (ws *WebsocketSession) onClientSignal(ctx context.Context, message []byte) (Event, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.onClientSignal")
	defer span.Finish()

	var event Event

	err := json.Unmarshal(message, &event)

	if err != nil {
		ext.LogError(span, err)
		return Event{}, err
	}

	span.SetTag("session", event.Session)
	span.SetTag("call", event.Call)
	span.SetTag("event", event.Kind)

	switch {
	case event.Kind != MessageKindJoin:
		err = errors.Errorf(`received unexpected event kind "%s"`, event.Kind)
	case event.Call == "":
		err = errors.Errorf("received event with empty call id")
	case event.Session == "":
		err = errors.Errorf("received event with empty session id")
	default:
		return event, nil
	}

	ext.LogError(span, err)
	return Event{}, err
}

func (ws *WebsocketSession) onPeerSignal(ctx context.Context, message []byte, call string, session string) (Event, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.onPeerSignal")
	defer span.Finish()

	var peer PeerEvent

	err := json.Unmarshal(message, &peer)

	if err != nil {
		ext.LogError(span, err)
		return Event{}, err
	}

	span.SetTag("call", call)
	span.SetTag("session", session)
	span.SetTag("event", peer.Kind)
	span.SetTag("peer.call", peer.Call)
	span.SetTag("peer.session", peer.Peer)

	switch {
	case peer.Call != call:
		err = errors.Errorf(`received unexpected event for call "%s"`, peer.Call)
	case peer.Peer == "":
		err = errors.New("received event missing peer ID")
	case peer.Kind != MessageKindPeer && peer.Kind != MessageKindOffer && peer.Kind != MessageKindAnswer:
		err = errors.Errorf(`received unexpected event kind "%s" from %s:%s`, peer.Kind, peer.Call, peer.Peer)
	case peer.Peer == session:
		// TODO this shouldn't be an error
		err = errors.Errorf("received echo event")
	default:
		return peer.Event, nil
	}

	ext.LogError(span, err)
	return Event{}, err
}
