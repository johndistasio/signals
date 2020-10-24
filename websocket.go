package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type WebsocketHandler struct {
	lock     Semaphore
	redis    Redis
	upgrader websocket.Upgrader
	opts     *WebsocketSessionOptions
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
		joinTimeout:  wh.opts.JoinTimeout,
		readTimeout:  wh.opts.ReadTimeout,
		pingInterval: wh.opts.PingInterval,
		conn:         conn,
	}

	go ws.Start()
}

type WebsocketSessionOptions struct {
	JoinTimeout  time.Duration
	ReadTimeout  time.Duration
	PingInterval time.Duration
}

// TODO document tight coupling of connection and goroutine lifecycles
type WebsocketSession struct {
	lock         Semaphore
	redis        Redis
	joinTimeout  time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration
	conn         *websocket.Conn
	pubsub       *redis.PubSub

	call    string
	session string
}

func (ws *WebsocketSession) close(message string) error {
	m := websocket.FormatCloseMessage(websocket.CloseNormalClosure, message)
	return ws.conn.WriteControl(websocket.CloseMessage, m, time.Now().Add(1*time.Second))
}

// This function handles the complexity of managing the websocket connection and shuttling messages to the client.
func (ws *WebsocketSession) Start() {
	span, ctx := opentracing.StartSpanFromContext(context.Background(), "WebsocketSession.Start")
	defer span.Finish()

	// Set an initial read deadline. The connection will be closed if we don't receive a handshake message within this
	// time frame.
	if ws.joinTimeout > 0 {
		err := ws.conn.SetReadDeadline(time.Now().Add(ws.joinTimeout))
		if err != nil {
			ext.LogError(span, err)
			_ = ws.close("socket error")
			return
		}
	}

	// Wait for a "join call" handshake message from the client.
	_, message, err := ws.conn.ReadMessage()

	if err != nil {
		ext.LogError(span, err)
		_ = ws.close("handshake read failure")
		return
	}

	var join Event

	err = json.Unmarshal(message, &join)

	if err != nil {
		span.LogFields(log.String("close", err.Error()))
		_ = ws.close("bad handshake")
		return
	}

	valid := ValidateClientHandshake(ctx, join)

	if !valid {
		span.LogFields(log.String("close", "bad handshake"))
		_ = ws.close("bad handshake")
		return
	}

	span.SetTag("event", join.Kind)
	span.SetTag("call", join.Call)
	span.SetTag("session", join.Session)

	// Check if the client has a valid seat on the call.
	held, err := ws.lock.Check(ctx, join.Call, join.Session)

	if err != nil {
		ext.LogError(span, err)
		_ = ws.close("backend failure")
		return
	}

	if !held {
		span.LogFields(log.String("close", "lock not held"))
		_ = ws.close("bad seat")
		return
	}

	ws.call = join.Call
	ws.session = join.Session

	// Subscribe to the signal publishing backend.
	ws.pubsub = ws.redis.Subscribe(ctx, RedisTopicPrefix+ws.call)
	_, err = ws.pubsub.Receive(ctx)

	if err != nil {
		ext.LogError(span, err)
		_ = ws.close("backend failure")
		return
	}

	// Client pongs are used to determine client liveliness.
	ws.conn.SetPongHandler(ws.pongHandler)

	bytes, _ := json.Marshal(Event{Kind: MessageKindWelcome, Call: ws.call})

	err = ws.conn.WriteMessage(websocket.TextMessage, bytes)

	if err != nil {
		ext.LogError(span, err)
		_ = ws.close("confirmation failure")
	}

	go ws.readLoop()
	go ws.writeLoop()
}

// pongHandler resets the read deadline and renews the client's lease on their seat. When the least can't be renewed,
// it will return an error which will cause the underlying implementation to close the websocket.
func (ws *WebsocketSession) pongHandler(_ string) error {
	span, ctx := opentracing.StartSpanFromContext(context.Background(), "WebsocketSession.pongHandler")
	defer span.Finish()

	span.SetTag("call", ws.call)
	span.SetTag("session", ws.session)

	// Reset the read deadline upon receiving a ping.
	if ws.readTimeout > 0 {
		_ = ws.conn.SetReadDeadline(time.Now().Add(ws.readTimeout))
	}

	if held, err := ws.lock.Acquire(ctx, ws.call, ws.session); err != nil || !held {
		message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "seat renewal failure")
		_ = ws.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(1*time.Second))
	}

	return nil
}

func ValidateClientHandshake(ctx context.Context, event Event) bool {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.ValidateClientHandshake")
	defer span.Finish()

	if  event.Call == "" {
		return false
	}

	if event.Session == "" {
		return false
	}

	if event.Body != "" {
		return false
	}

	if event.Kind != MessageKindJoin {
		return false
	}

	return true
}

// ValidatePeerEvent vets events from peers for invalid conditions.
func ValidatePeerEvent(ctx context.Context, call string, event Event) bool {
	span, _ := opentracing.StartSpanFromContext(ctx, "WebsocketSession.ValidatePeerEvent")
	defer span.Finish()

	if event.Call != call {
		return false
	}

	if event.Session == "" {
		return false
	}

	if event.Kind != MessageKindPeerJoin && event.Kind != MessageKindOffer && event.Kind != MessageKindAnswer {
		return false
	}

	if (event.Kind == MessageKindOffer || event.Kind == MessageKindAnswer) && event.Body == "" {
		return false
	}

	return true
}

// Continuously reads to ensure that control messages are processed. After the initial handshake we don't care
// about messages from the client so those are discarded. The websocket library handles control messages internally.
func (ws *WebsocketSession) readLoop() {
	// If the read returns an error then the socket is in a bad state (e.g. the read timeout expired) and we won't
	// be able to process control messages anymore. If this happens we should abort the entire connection and leave
	// it to clients to reconnect if needed.
	//
	// Calling this in a defer ensure that this happens even if the reader panics.
	defer func () {
		_ = ws.close("websocket reader stopping")
	}()

	for {
		if _, _, err := ws.conn.NextReader(); err != nil {
			return
		}
	}
}

func (ws *WebsocketSession) writeLoop() {
	// Initialize a ticket for sending ping messages to the client; we'll use this to detect dead clients.
	ticker := time.NewTicker(ws.pingInterval)

	// TODO rewrite
	// Clean up resources upon returning from this function. Any goroutines spawned by this function should tie their
	// lifecycle to the connection or the pubsub so as to avoid leaks.
	defer func() {
		ticker.Stop()
		_ = ws.pubsub.Close()
		_ = ws.close("websocket writer stopping")
	}()

	// This channel will close when the pubsub is closed, either explicitly with a Close() or due to connection loss.
	ch := ws.pubsub.Channel()

	for {
		select {
		case <-ticker.C:
			// TODO trace
			err := ws.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(1*time.Second))

			if err != nil {
				return
			}
		case msg, ok := <-ch:
			if !ok {
				return
			}

			bytes := []byte(msg.Payload)

			if err := ws.onPeerEvent(bytes); err != nil {
				return
			}
		}
	}
}

func (ws *WebsocketSession) onPeerEvent(message []byte) error {
	span, ctx := opentracing.StartSpanFromContext(context.Background(), "WebsocketSession.onPeerMessage")
	defer span.Finish()

	span.SetTag("call", ws.call)
	span.SetTag("session", ws.session)

	var peer Event

	err := json.Unmarshal(message, &peer)

	if err != nil {
		ext.LogError(span, err)
		return err
	}

	span.SetTag("event", peer.Kind)
	span.SetTag("peer.call", peer.Call)
	span.SetTag("peer.session", peer.Session)

	valid := ValidatePeerEvent(ctx, ws.call, peer)

	if !valid {
		span.LogFields(log.String("skip", "invalid event"))
		return nil
	}

	held, err := ws.lock.Check(ctx, ws.call, ws.session)

	if err != nil {
		ext.LogError(span, err)
		return err
	}

	if !held {
		return errors.New("invalid session")
	}

	// Don't send echo'd messages back to clients.
	if peer.Session == ws.session {
		span.LogFields(log.String("skip", "echo event"))
		return nil
	}

	bytes, err := json.Marshal(peer)

	if err != nil {
		ext.LogError(span, err)
		return err
	}

	err = ws.conn.WriteMessage(websocket.TextMessage, bytes)

	if err != nil {
		ext.LogError(span, err)
		return err
	}

	return nil
}

