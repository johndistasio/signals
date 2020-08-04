package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
	"log"
	"strings"
	"time"
)

// TODO allow for arbitrary rooms
var room = "room:test"
var channel = "channel:test"


// TODO replace with members
var PongTimeout = 60 * time.Second
var WriteTimeout = 10 * time.Second
var PingInterval = 30 * time.Second

type SignalRelay struct {
	// Max time between received pongs. If this timeout elapses, we consider the client to be dead.
	PongTimeout time.Duration

	// Max time for message writes to the client. If this timeout elapses, we consider the client to be dead.
	WriteTimeout time.Duration

	// Time between pings sent to the client. Must be less than PongTimeout to give the client a chance to respond.
	PingInterval time.Duration

	id string

	ctx context.Context

	cancel context.CancelFunc

	conn *websocket.Conn

	rdb *redis.Client
}

type Signal struct {
	PeerId string
	Message string
}

func (r *SignalRelay) stop() {
	if r.ctx.Err() == nil {
		r.cancel()
	}

	_ = r.conn.Close()
}

func StartSignalRelay(ctx context.Context, id string, rdb *redis.Client, conn *websocket.Conn) {
	ctx, cancel := context.WithCancel(ctx)

	relay := &SignalRelay{
		PongTimeout: PongTimeout,
		WriteTimeout: WriteTimeout,
		PingInterval: PingInterval,
		id: id,
		ctx: ctx,
		cancel: cancel,
		rdb: rdb,
		conn: conn,
	}

	conn.SetCloseHandler(func(code int, text string) error {
		log.Println("close handler firing")
		relay.stop()
		return nil
	})

	conn.SetPongHandler(func(appData string) error {
		log.Println("pong handler firing")

		err := conn.SetReadDeadline(time.Now().Add(PongTimeout))

		if err != nil {
			// If this errors then the underlying network connection is borked.
			log.Println("error on SetReadDeadline")
			relay.stop()
		}

		// TODO reset peering key expiration

		return err
	})

	// TODO: set up read/write size and time limits

	go relay.ReadSignal()
	go relay.WriteSignal()
}

func (r *SignalRelay) ReadSignal() {
	defer r.stop()

	for {
		select {
		case <-r.ctx.Done():
			log.Println("ReadSignal stopping on relay shutdown")
			return
		default:
			// TODO tracing
			ctx := context.Background()

			// On socket close, this will return an error.
			_, message, err := r.conn.ReadMessage()

			if err != nil {
				if _, ok := err.(*websocket.CloseError); ok {
					log.Println("reader stopping on closed websocket")
					// Halt on socket close.
					return
				}

				if strings.Contains(err.Error(), "use of closed network connection") {
					log.Println("reader stopping on closed socket")
					// Halt on socket close.
					// There doesn't seem to be a better way to detect this error from Go's TCP library. Their own
					// HTTP/2 code does this exact string comparison when checking for a socket close, too.
					return
				}

				log.Printf("reader error on ReadMessage: %v\n", err)

				// TODO alert the client of failure

				// TODO are there other error cases when we'd like to halt the reader here?
				continue
			}

			// TODO handle error
			msg, _ := json.Marshal(Signal{r.id, string(message)})

			err = rdb.Publish(ctx, channel, msg).Err()

			if err != nil {
				log.Printf("error on publish to redis: %v\n", err)

				// TODO add backoff, and figure out how to alert the client of successive failures
			}
		}
	}
}

func (r *SignalRelay) WriteSignal() {
	ping := time.NewTicker(PingInterval)

	// TODO subscription needs it's own context and setup function
	ch := rdb.Subscribe(r.ctx, channel)

	defer func() {
		_ = ch.Close()
		ping.Stop()
		r.stop()
	}()

	for {
		select {
		case <-r.ctx.Done():
			log.Printf("%s: WriteSignal stopping on relay shutdown\n", r.id)
			return
		case <-ping.C:
			if err := r.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: writer error on WriteMessage (pinger): %v\n", r.id, err)
				}

				return
			}
		case message := <-ch.Channel():

			bMessage := []byte(message.Payload)

			var signal Signal

			// TODO handle error
			_ = json.Unmarshal(bMessage, &signal)

			if signal.PeerId == r.id {
				continue
			}

			err := r.conn.WriteMessage(websocket.TextMessage, bMessage)

			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("%s: unexpected error on WriteMessage: %v\n", r.id, err)
				}

				return
			}
		default:
			// intentionally does nothing
		}
	}
}